#!/bin/bash

# Before the script can be executed, the following must be done:

# * Create IAM user. TODO(mp) write about policies and friends.

set -e
set -u
#set -x

: ${IMAGE_ID:="ami-fce3c696"} # Ubuntu 14.04 LTS (HVM) at us-east-1
: ${ZONE:="us-east-1a"}
: ${INSTANCE_TYPE:="t2.micro"}
: ${KEY_NAME:="weavenet_ci"}
: ${SSH_KEY_FILE:="$HOME/.ssh/$KEY_NAME"}
: ${NUM_HOSTS:=5}
: ${SEC_GROUP_NAME:="weavenet-ci"}
: ${SUFFIX:=""}
: ${AWSCLI:="aws"}
: ${SSH:="ssh -o StrictHostKeyChecking=no -o CheckHostIp=no
             -o UserKnownHostsFile=/dev/null -l ubuntu -i $SSH_KEY_FILE"}

# Creates and runs a set of VMs.
# Each VM is named after "host${ID}${SUFFIX}".
function setup {
    # Destroy previous machines (if any)
    destroy

    # Create keypair
    create_key_pair

    # Check whether a necessary security group exists
    ensure_sec_group

    # Start instances
    $AWSCLI ec2 run-instances                   \
        --image-id "$IMAGE_ID"                  \
        --key-name "$KEY_NAME"                  \
        --placement "AvailabilityZone=$ZONE"    \
        --instance-type "$INSTANCE_TYPE"        \
        --security-groups "$SEC_GROUP_NAME"     \
        --count "$NUM_HOSTS"

    # Assign a name to each instance and
    # disable src/dst checks (required by awsvpc)
    json=$(mktemp json.XXXXXXXXXX)
    list_instances > $json
    i=1
    for vm in `jq -r ".Reservations[].Instances[].InstanceId" $json`; do
        $AWSCLI ec2 create-tags                             \
            --resources "$vm"                               \
            --tags "Key=Name,Value=\"$(vm_name $i)\""
        $AWSCLI ec2 modify-instance-attribute               \
            --instance-id "$vm"                             \
            --no-source-dest-check
        ((i++))
    done

    # Populate /etc/hosts of local host and of each instance
	hosts=$(mktemp hosts.XXXXXXXXXX)
    list_instances > $json
    names=$(vm_names)
    for vm in $names; do
		echo "$(internal_ip $json $vm) $vm" >> $hosts
    done
    for vm in $names; do
		sudo sed -i "/$vm/d" /etc/hosts
		sudo sh -c "echo \"$(external_ip $json $vm) $vm\" >>/etc/hosts"
		try_connect $vm
		copy_hosts $vm $hosts &
	    install_docker_on $vm &
    done

	wait

    rm $json $hosts
}

# Creates AMI.
function make_template {
    echo "NYI"
}

# Destroy VMs and remove keys.
function destroy {
    delete_key_pair
    json=$(mktemp json.XXXXXXXXXX)
    list_instances >> $json
    instances=""
    for i in `jq -r ".Reservations[].Instances[].InstanceId" $json`; do
        instances="$i $instances"
    done

    [[ ! -z "$instances" ]] &&
        $AWSCLI ec2 terminate-instances --instance-ids $instances > /dev/null

    rm $json
}

# Helpers

function list_instances {
    $AWSCLI ec2 describe-instances                                      \
        --filters "Name=image-id,Values=$IMAGE_ID"                      \
                  "Name=instance-state-name,Values=pending,running"
}

function get_instance_id_by_name {
    jq -r ".Reservations[].Instances[]
          | select (.Tags[].Value == \"$2\")
          | .InstanceId" $1
}

function vm_names {
	local names=
	for i in $(seq 1 $NUM_HOSTS); do
        names="$(vm_name $i) $names"
	done
	echo "$names"
}

function vm_name {
    id="$1"
    echo "host$id$SUFFIX"
}

function internal_ip {
    jq -r ".Reservations[].Instances[]
          | select (.Tags[0].Key == \"Name\" and .Tags[0].Value == \"$2\")
          | .NetworkInterfaces[0].PrivateIpAddress" $1
}

function external_ip {
    jq -r ".Reservations[].Instances[]
          | select (.Tags[0].Key == \"Name\" and .Tags[0].Value == \"$2\")
          | .NetworkInterfaces[0].Association.PublicIp" $1
}

function create_key_pair {
    function _create {
        $AWSCLI ec2 create-key-pair --key-name $KEY_NAME 2>&1
    }

    if ! RET=$(_create); then
        if echo "$RET" | grep -q "InvalidKeyPair\.Duplicate"; then
            echo "Deleting key pair"
            delete_key_pair
            RET=$(_create)
        else
            echo "$RET"
            exit -1
        fi
    fi

    echo "$RET" | jq -r .KeyMaterial > $SSH_KEY_FILE
    chmod 400 $SSH_KEY_FILE
}

function delete_key_pair {
    $AWSCLI ec2 delete-key-pair --key-name $KEY_NAME
    rm -f "$SSH_KEY_FILE" || true
}

function ensure_sec_group {
    $AWSCLI ec2 describe-security-groups |                              \
        jq -r -e ".SecurityGroups[] |
                select (.GroupName == \"$SEC_GROUP_NAME\")" > /dev/null \
        || create_sec_group
}

function create_sec_group {
    $AWSCLI ec2 create-security-group               \
        --group-name "$SEC_GROUP_NAME"              \
        --description "Weave CircleCI" > /dev/null
    $AWSCLI ec2 authorize-security-group-ingress    \
        --group-name "$SEC_GROUP_NAME"              \
        --source-group "$SEC_GROUP_NAME"            \
        --protocol all
    $AWSCLI ec2 authorize-security-group-ingress    \
        --group-name "$SEC_GROUP_NAME"              \
        --protocol tcp --port 22                    \
        --cidr "0.0.0.0/0"
    $AWSCLI ec2 authorize-security-group-ingress    \
        --group-name "$SEC_GROUP_NAME"              \
        --protocol tcp --port 2375                  \
        --cidr "0.0.0.0/0"
}

# Commons (taken from gce.sh, and slightly modified)

# TODO(mp) DRY

function hosts {
	hosts=
	json=$(mktemp json.XXXXXXXXXX)
	list_instances > $json
	for name in $(vm_names); do
		hostname="$name"
		hosts="$hostname $hosts"
	done
	echo export SSH=\"$SSH\"
	echo export HOSTS=\"$hosts\"
	rm $json
}

function try_connect {
    echo "trying"
	for i in {0..10}; do
		$SSH -t $1 true && return
		sleep 2
	done
    echo "connected"
}

function copy_hosts {
	hostname=$1
	hosts=$2
	cat $hosts | $SSH -t "$hostname" "sudo -- sh -c \"cat >>/etc/hosts\""
}

function install_docker_on {
    # TODO(mp) bring back `-s overlay` opt to DOCKER_OPTS.
    # TODO(mp) *maybe* use `vagrant` user instead of default `ubuntu`.

	name=$1
	$SSH -t $name sudo bash -x -s <<EOF
curl -sSL https://get.docker.com/gpg | sudo apt-key add -
curl -sSL https://get.docker.com/ | sh
apt-get update -qq;
apt-get install -q -y --force-yes --no-install-recommends ethtool;
usermod -a -G docker ubuntu;
echo 'DOCKER_OPTS="-H unix:///var/run/docker.sock -H unix:///var/run/alt-docker.sock -H tcp://0.0.0.0:2375"' >> /etc/default/docker;
service docker restart
EOF
	# It seems we need a short delay for docker to start up, so I put this in
	# a separate ssh connection.  This installs nsenter.
	$SSH -t $name sudo docker run --rm -v /usr/local/bin:/target jpetazzo/nsenter
}

# Main

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
    ;;
esac
