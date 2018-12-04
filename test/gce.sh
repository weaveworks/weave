#!/bin/bash
# This script has a bunch of GCE-related functions:
# ./gce.sh setup -  starts two VMs on GCE and configures them to run our integration tests
# . ./gce.sh; ./run_all.sh - set a bunch of environment variables for the tests
# ./gce.sh destroy - tear down the VMs
# ./gce.sh make_template - make a fresh VM template; update TEMPLATE_NAME first!

set -e

: ${KEY_FILE:=/tmp/gce_private_key.json}
: ${SSH_KEY_FILE:=$HOME/.ssh/gce_ssh_key}
: ${PROJECT:=positive-cocoa-90213}
: ${IMAGE_FAMILY:=ubuntu-1604-lts}
: ${IMAGE_PROJECT:=ubuntu-os-cloud}
: ${TEMPLATE_NAME:=test-template-12}
: ${ZONE:=us-central1-a}
: ${NUM_HOSTS:=5}
SUFFIX=""
if [ -n "$CIRCLECI" ]; then
	SUFFIX="-${CIRCLE_BUILD_NUM}-$CIRCLE_NODE_INDEX"
fi

# Setup authentication
gcloud auth activate-service-account --key-file $KEY_FILE 1>/dev/null
gcloud config set project $PROJECT

function vm_names {
	local names=
	for i in $(seq 1 $NUM_HOSTS); do
		names="host$i$SUFFIX $names"
	done
	echo "$names"
}

# Delete all vms in this account
function destroy {
	if [ $(gcloud compute firewall-rules list test-allow-docker$SUFFIX 2>/dev/null | wc -l) -gt 0 ] ; then
		gcloud compute firewall-rules delete test-allow-docker$SUFFIX
	fi
	names="$(vm_names)"
	if [ $(gcloud compute instances list --zones $ZONE -q $names | wc -l) -le 1 ] ; then
		return 0
	fi
	for i in {0..10}; do
		# gcloud instances delete can sometimes hang.
		case  $(set +e; timeout 60s /bin/bash -c "gcloud compute instances delete --zone $ZONE -q $names  >/dev/null 2>&1"; echo $?) in
			0)
				return 0
				;;
			124)
				# 124 means it timed out
				break
				;;
			*)
				return 1
		esac
	done
}

function internal_ip {
	jq -r ".[] | select(.name == \"$2\") | .networkInterfaces[0].networkIP" $1
}

function external_ip {
	jq -r ".[] | select(.name == \"$2\") | .networkInterfaces[0].accessConfigs[0].natIP" $1
}

function try_connect {
	for i in {0..10}; do
		ssh -t $1 true && return
		sleep 2
	done
}

function install_docker_on {
	name=$1
	ssh -t $name sudo bash -x -s <<EOF
curl -sSL https://get.docker.com/gpg | sudo apt-key add -
curl -sSL https://get.docker.com/ | sed -e s/docker-engine/docker-engine=1.11.2-0~xenial/ | sh
apt-get update -qq;
apt-get install -q -y --force-yes --no-install-recommends ethtool;
apt-get install -q -y bc jq;
usermod -a -G docker vagrant;
mkdir -p /etc/systemd/system/docker.service.d
cat >/etc/systemd/system/docker.service.d/override.conf  <<OVERRIDE
[Service]
ExecStart=
ExecStart=/usr/bin/docker daemon -H fd:// -H unix:///var/run/alt-docker.sock -H tcp://0.0.0.0:2375 -s overlay
OVERRIDE
systemctl daemon-reload
systemctl restart docker
# This installs nsenter.
docker run --rm -v /usr/local/bin:/target jpetazzo/nsenter
docker pull alpine
docker pull aanand/docker-dnsutils
docker pull weaveworks/hello-world
EOF
}

function install_kubernetes_on {
	name=$1
	ssh -t $name sudo bash -x -s <<EOF
curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
echo "deb http://apt.kubernetes.io/ kubernetes-xenial main" > /etc/apt/sources.list.d/kubernetes.list
apt-get update -qq
apt-get install -q -y kubelet kubeadm kubectl kubernetes-cni
systemctl --now disable kubelet
# Pre-pull images required for Kubernetes
docker pull gcr.io/google_containers/etcd-amd64:2.2.5
docker pull gcr.io/google_containers/kube-apiserver-amd64:v1.4.0
docker pull gcr.io/google_containers/kube-controller-manager-amd64:v1.4.0
docker pull gcr.io/google_containers/kube-proxy-amd64:v1.4.0
docker pull gcr.io/google_containers/kube-scheduler-amd64:v1.4.0
docker pull gcr.io/google_containers/kube-discovery-amd64:1.0
docker pull gcr.io/google_containers/pause-amd64:3.0
EOF
}

function copy_hosts {
	hostname=$1
	hosts=$2
	cat $hosts | ssh -t "$hostname" "sudo -- sh -c \"cat >>/etc/hosts\""
}

# Create new set of VMs
function setup {
	destroy

	names="$(vm_names)"
	gcloud compute instances create $names --image $TEMPLATE_NAME --zone $ZONE --tags test$SUFFIX --network=test
	my_ip="$(curl -s http://ipinfo.io/ip)"
	gcloud compute firewall-rules create test-allow-docker$SUFFIX --network=test --allow tcp:2375,tcp:12375 --target-tags test$SUFFIX --source-ranges $my_ip
	gcloud compute config-ssh --ssh-key-file $SSH_KEY_FILE
	sed -i '/UserKnownHostsFile=\/dev\/null/d' ~/.ssh/config

	# build an /etc/hosts file for these vms
	hosts=$(mktemp hosts.XXXXXXXXXX)
	json=$(mktemp json.XXXXXXXXXX)
	gcloud compute instances list --format=json >$json
	for name in $names; do
		echo "$(internal_ip $json $name) $name.$ZONE.$PROJECT" >>$hosts
	done

	for name in $names; do
		hostname="$name.$ZONE.$PROJECT"

		# Add the remote ip to the local /etc/hosts
		sudo sed -i "/$hostname/d" /etc/hosts
		sudo sh -c "echo \"$(external_ip $json $name) $hostname\" >>/etc/hosts"
		try_connect $hostname

		copy_hosts $hostname $hosts &
	done

	wait

	rm $hosts $json
}

function make_template {
	gcloud compute instances create $TEMPLATE_NAME --image-family=$IMAGE_FAMILY --image-project=$IMAGE_PROJECT --zone $ZONE
	gcloud compute config-ssh --ssh-key-file $SSH_KEY_FILE
	name="$TEMPLATE_NAME.$ZONE.$PROJECT"
	try_connect $name
	install_docker_on $name
	install_kubernetes_on $name
	gcloud -q compute instances delete $TEMPLATE_NAME --keep-disks boot --zone $ZONE
	gcloud compute images create $TEMPLATE_NAME --source-disk $TEMPLATE_NAME --source-disk-zone $ZONE
}

function hosts {
	hosts=
	json=$(mktemp json.XXXXXXXXXX)
	gcloud compute instances list --format=json >$json
	for name in $(vm_names); do
		hostname="$name.$ZONE.$PROJECT"
		hosts="$hostname $hosts"
	done
	echo export SSH=\"ssh -l vagrant\"
	echo export HOSTS=\"$hosts\"
	rm $json
}

case "$1" in
setup)
	setup
	;;

hosts)
	hosts
	;;

destroy)
	destroy
	;;

make_template)
	# see if template exists
	if ! gcloud compute images list | grep $PROJECT | grep $TEMPLATE_NAME; then
		make_template
	fi
    ;;

*)
	echo "Unknown command:" $1 >&2
    exit 1
esac
