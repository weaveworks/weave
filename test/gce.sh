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
ZONE=us-central1-a
NUM_HOSTS=2

# Setup authentication
gcloud auth activate-service-account --key-file $KEY_FILE
gcloud config set project $PROJECT

# Delete all vms in this account
function destroy {
	names="$(gcloud compute instances list --format=yaml | grep "^name\:" | cut -d: -f2 | xargs echo)"
	if [ -n "$names" ]; then
		gcloud compute instances delete --zone $ZONE -q $names
	fi
}

function external_ip {
	gcloud compute instances list $1 --format=yaml | grep "^    natIP\:" | cut -d: -f2 | tr -d ' '
}

function internal_ip {
	gcloud compute instances list $1 --format=yaml | grep "^  networkIP\:" | cut -d: -f2 | tr -d ' '
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
	names=
	for i in $(seq 1 $NUM_HOSTS); do
		names="host$i $names"
	done

	gcloud compute instances create $names --image $IMAGE --zone $ZONE
	gcloud compute config-ssh --ssh-key-file $SSH_KEY_FILE

	hosts=
	for i in $(seq 1 $NUM_HOSTS); do
		name="host$i.$ZONE.$PROJECT"
		install_docker_on $name

		# Add the remote ip to the local /etc/hosts
		sudo -- sh -c "echo \"$(external_ip host$i) $name\" >>/etc/hosts"
		# Add the local ips to the remote /etc/hosts
		for j in $(seq 1 $NUM_HOSTS); do
			ipaddr=$(internal_ip host$j)
			othername="host$j.$ZONE.$PROJECT"
			entry="$ipaddr $othername"
			ssh -t $name "sudo -- sh -c \"echo \\\"$entry\\\" >>/etc/hosts\""
		done
	done
}

function hosts {
	hosts=
	args=
	for i in $(seq 1 $NUM_HOSTS); do
		name="host$i.$ZONE.$PROJECT"
		hosts="$name $hosts"
		args="--add-host=$name:$(internal_ip host$i) $args"
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
esac
