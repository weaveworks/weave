#!/bin/bash
# This script has a bunch of GCE-related functions:
# ./gce.sh setup -  starts two VMs on GCE and configures them to run our integration tests
# . ./gce.sh; ./run_all.sh - set a bunch of environment variables for the tests
# ./gce.sh destroy - tear down the VMs

set -ex

KEY_FILE=/tmp/gce_private_key.json
SSH_KEY_FILE=$HOME/.ssh/gce_ssh_key
PROJECT=positive-cocoa-90213
IMAGE=ubuntu-14-04
TEMPLATE_NAME="test-template"
ZONE=us-central1-a
NUM_HOSTS=2
SUFFIX=""
if [ -n "$CIRCLECI" ]; then
	SUFFIX="-${CIRCLE_BUILD_NUM}-$CIRCLE_NODE_INDEX"
fi

# Setup authentication
gcloud auth activate-service-account --key-file $KEY_FILE
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
	for i in {0..10}; do
		# gcloud instances delete can sometimes hang.
		timeout 60s /bin/bash -c "gcloud compute instances delete --zone $ZONE -q $names || true"
		# 124 means it timed out
		if [ $? -ne 124 ]; then
			return
		fi
	done
}

function external_ip {
	gcloud compute instances list $1 --format=yaml | grep "^    natIP\:" | cut -d: -f2 | tr -d ' '
}

function internal_ip {
	gcloud compute instances list $1 --format=yaml | grep "^  networkIP\:" | cut -d: -f2 | tr -d ' '
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
apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9;
echo deb https://get.docker.io/ubuntu docker main > /etc/apt/sources.list.d/docker.list;
apt-get update -qq;
apt-get install -q -y --force-yes --no-install-recommends lxc-docker ethtool;
usermod -a -G docker vagrant;
echo 'DOCKER_OPTS="-H unix:///var/run/docker.sock -H tcp://0.0.0.0:2375"' >> /etc/default/docker;
service docker restart
EOF
	# It seems we need a short delay for docker to start up, so I put this in
	# a separate ssh connection.  This installs nsenter.
	ssh -t $name sudo docker run --rm -v /usr/local/bin:/target jpetazzo/nsenter
}

# Create new set of VMS
function setup {
	destroy
	names="$(vm_names)"
	gcloud compute instances create $names --image $TEMPLATE_NAME --zone $ZONE
	gcloud compute config-ssh --ssh-key-file $SSH_KEY_FILE

	for name in $names; do
		hostname="$name.$ZONE.$PROJECT"
		# Add the remote ip to the local /etc/hosts
		sudo -- sh -c "echo \"$(external_ip $name) $hostname\" >>/etc/hosts"
		try_connect $hostname

		# Add the local ips to the remote /etc/hosts
		for othername in $names; do
			entry="$(internal_ip $othername) $othername.$ZONE.$PROJECT"
			ssh -t "$hostname" "sudo -- sh -c \"echo \\\"$entry\\\" >>/etc/hosts\""
		done
	done
}

function make_template {
	gcloud compute instances create template --image $IMAGE --zone $ZONE
	gcloud compute config-ssh --ssh-key-file $SSH_KEY_FILE
	name="template.$ZONE.$PROJECT"
	install_docker_on $name
	gcloud compute instances delete template --keep-disks boot -zone $ZONE
	gcloud compute images create $TEMPLATE_NAME --source-disk example-disk --source-disk-zone $ZONE
}

function hosts {
	hosts=
	args=
	for name in $(vm_names); do
		hostname="$name.$ZONE.$PROJECT"
		hosts="$hostname $hosts"
		args="--add-host=$hostname:$(internal_ip $name) $args"
	done
	export SSH=ssh
	export HOSTS="$hosts"
	export WEAVE_DOCKER_ARGS="$args"
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
	make_template
esac
