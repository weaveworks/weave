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
: ${TEMPLATE_NAME:=test-template-11}
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
usermod -a -G docker vagrant;
echo 'DOCKER_OPTS="-H unix:///var/run/docker.sock -H unix:///var/run/alt-docker.sock -H tcp://0.0.0.0:2375 -s overlay"' >> /etc/default/docker;
service docker restart
EOF
	# It seems we need a short delay for docker to start up, so I put this in
	# a separate ssh connection.  This installs nsenter.
	ssh -t $name sudo docker run --rm -v /usr/local/bin:/target jpetazzo/nsenter
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
	gcloud compute instances create $names --image $TEMPLATE_NAME --zone $ZONE
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
esac
