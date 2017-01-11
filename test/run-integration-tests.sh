#!/bin/bash
#
# Description:
#   This script runs all Weave Net's integration tests on the specified
#   provider (default: Google Cloud Platform).
#
# Usage:
#
#   Run all integration tests on Google Cloud Platform:
#   $ ./run-integration-tests.sh
#
#   Run all integration tests on Amazon Web Services:
#   PROVIDER=aws ./run-integration-tests.sh
#

set -e
DIR="$(dirname "$0")"
. "$DIR/../tools/provisioning/setup.sh" # Import gcp_on, do_on, and aws_on.
. "$DIR/config.sh"                      # Import greenly.

# Variables:
APP="weave-net"
# shellcheck disable=SC2034
PROJECT="weave-net-tests" # Only used when PROVIDER is gcp, by tools/provisioning/config.sh.
NAME=${NAME:-"$(whoami | sed -e 's/[\.\_]*//g' | cut -c 1-4)"}
PROVIDER=${PROVIDER:-gcp} # Provision using provided provider, or Google Cloud Platform by default.
NUM_HOSTS=${NUM_HOSTS:-10}
PLAYBOOK=${PLAYBOOK:-setup_weave-net_test.yml}
TESTS=${TESTS:-}
RUNNER_ARGS=${RUNNER_ARGS:-""}
# Dependencies' versions:
DOCKER_VERSION=${DOCKER_VERSION:-1.11.2}
KUBERNETES_VERSION=${KUBERNETES_VERSION:-1.5.2}
KUBERNETES_CNI_VERSION=${KUBERNETES_CNI_VERSION:-0.3.0.1}
# Lifecycle flags:
SKIP_CONFIG=${SKIP_CONFIG:-}

function print_vars() {
    echo "--- Variables: Main ---"
    echo "PROVIDER=$PROVIDER"
    echo "NUM_HOSTS=$NUM_HOSTS"
    echo "PLAYBOOK=$PLAYBOOK"
    echo "TESTS=$TESTS"
    echo "SSH_OPTS=$SSH_OPTS"
    echo "RUNNER_ARGS=$RUNNER_ARGS"
    echo "--- Variables: Versions ---"
    echo "DOCKER_VERSION=$DOCKER_VERSION"
    echo "KUBERNETES_VERSION=$KUBERNETES_VERSION"
    echo "KUBERNETES_CNI_VERSION=$KUBERNETES_CNI_VERSION"
    echo "--- Variables: Flags ---"
    echo "SKIP_CONFIG=$SKIP_CONFIG"
}

function verify_dependencies() {
    local deps=(python terraform ansible-playbook)
    for dep in "${deps[@]}"; do
        if [ ! "$(which "$dep")" ]; then
            echo >&2 "$dep is not installed or not in PATH."
            exit 1
        fi
    done
}

# shellcheck disable=SC2155
function provision_locally() {
    export VAGRANT_CWD="$(dirname "${BASH_SOURCE[0]}")"
    case "$1" in
        on)
            vagrant up
            local status=$?

            # Set up SSH connection details: 
            local ssh_config=$(mktemp /tmp/vagrant_ssh_config_XXX)
            vagrant ssh-config >"$ssh_config"
            export SSH="ssh -F $ssh_config"
            # Extract username, SSH private key, and VMs' IP addresses:
            ssh_user="$(sed -ne 's/\ *User //p' "$ssh_config" | uniq)"
            ssh_id_file="$(sed -ne 's/\ *IdentityFile //p' "$ssh_config" | uniq)"
            ssh_hosts=$(sed -ne 's/Host //p' "$ssh_config")

            # Set up /etc/hosts files on this ("local") machine and the ("remote") testing machines, to map hostnames and IP addresses, so that:
            # - this machine communicates with the testing machines via their public IPs;
            # - testing machines communicate between themselves via their private IPs;
            # - we can simply use just the hostname in all scripts to refer to machines, and the difference between public and private IP becomes transparent.
            # N.B.: if you decide to use public IPs everywhere, note that some tests may fail (e.g. test #115).
            update_local_etc_hosts "$ssh_hosts" "$(for host in $ssh_hosts; do $SSH "$host" "cat /etc/hosts | grep $host"; done)"

            SKIP_CONFIG=1 # Vagrant directly configures virtual machines using Ansible -- see also: Vagrantfile
            return $status
            ;;
        off)
            vagrant destroy -f
            ;;
        *)
            echo >&2 "Unknown command $1. Usage: {on|off}."
            exit 1
            ;;
    esac
}

function update_local_etc_hosts() {
    echo "> Updating local /etc/hosts..."
    # Remove old entries (if present):
    for host in $1; do sudo sed -i "/$host/d" /etc/hosts; done
    # Add new entries:
    sudo sh -c "echo \"$2\" >> /etc/hosts"
}

function upload_etc_hosts() {
    # Remove old entries (if present):
    # shellcheck disable=SC2016,SC2086
    $SSH $3 'for host in '$1'; do sudo sed -i "/$host/d" /etc/hosts; done'
    # Add new entries:
    echo "$2" | $SSH "$3" "sudo -- sh -c \"cat >> /etc/hosts\""
}

function update_remote_etc_hosts() {
    echo "> Updating remote /etc/hosts..."
    local pids=""
    for host in $1; do
        upload_etc_hosts "$1" "$2" "$host" &
        local pids="$pids $!"
    done
    for pid in $pids; do wait "$pid"; done
}

function provision_remotely() {
    case "$1" in
        on)
            terraform apply -input=false -parallelism="$NUM_HOSTS" -var "app=$APP" -var "name=$NAME" -var "num_hosts=$NUM_HOSTS" "$DIR/../tools/provisioning/$2"
            local status=$?
            ssh_user=$(terraform output username)
            ssh_id_file=$(terraform output private_key_path)
            ssh_hosts=$(terraform output hostnames)
            export SSH="ssh -l $ssh_user -i $ssh_id_file $SSH_OPTS"

            # Set up /etc/hosts files on this ("local") machine and the ("remote") testing machines, to map hostnames and IP addresses, so that:
            # - this machine communicates with the testing machines via their public IPs;
            # - testing machines communicate between themselves via their private IPs;
            # - we can simply use just the hostname in all scripts to refer to machines, and the difference between public and private IP becomes transparent.
            # N.B.: if you decide to use public IPs everywhere, note that some tests may fail (e.g. test #115).
            update_local_etc_hosts "$ssh_hosts" "$(terraform output public_etc_hosts)"
            update_remote_etc_hosts "$ssh_hosts" "$(terraform output private_etc_hosts)"

            return $status
            ;;
        off)
            terraform destroy -force "$DIR/../tools/provisioning/$2"
            ;;
        *)
            echo >&2 "Unknown command $1. Usage: {on|off}."
            exit 1
            ;;
    esac
}

# shellcheck disable=SC2155
function provision() {
    local action=$([ "$1" == "on" ] && echo "Provisioning" || echo "Shutting down")
    echo
    greenly echo "> $action test host(s) on [$PROVIDER]..."
    local begin_prov=$(date +%s)
    case "$2" in
        'aws')
            aws_on
            provision_remotely "$1" "$2"
            ;;
        'do')
            do_on
            provision_remotely "$1" "$2"
            ;;
        'gcp')
            gcp_on
            provision_remotely "$1" "$2"
            ;;
        'vagrant')
            provision_locally "$1"
            ;;
        *)
            echo >&2 "Unknown provider $2. Usage: PROVIDER={gcp|aws|do|vagrant}."
            exit 1
            ;;
    esac

    echo
    greenly echo "> Provisioning took $(date -u -d @$(($(date +%s) - begin_prov)) +"%T")."
}

# shellcheck disable=SC2155
function configure() {
    echo
    if [ -n "$SKIP_CONFIG" ]; then
        greenly echo "> Skipped configuration of test host(s)."
    else
        greenly echo "> Configuring test host(s)..."
        local begin_conf=$(date +%s)
        local inventory_file=$(mktemp /tmp/ansible_inventory_XXXXX)
        echo "[all]" >"$inventory_file"
        # shellcheck disable=SC2001
        echo "$2" | sed "s/$/:$3/" >>"$inventory_file"

        # Configure the provisioned machines using Ansible, allowing up to 3 retries upon failure (e.g. APT connectivity issues, etc.):
        for i in $(seq 3); do
            ansible-playbook -u "$1" -i "$inventory_file" --private-key="$4" --forks="$NUM_HOSTS" \
                --ssh-extra-args="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null" \
                --extra-vars "docker_version=$DOCKER_VERSION kubernetes_version=$KUBERNETES_VERSION kubernetes_cni_version=$KUBERNETES_CNI_VERSION" \
                "$DIR/../tools/config_management/$PLAYBOOK" \
                && break || echo >&2 "#$i: Ansible failed. Retrying now..."
        done

        echo
        greenly echo "> Configuration took $(date -u -d @$(($(date +%s) - begin_conf)) +"%T")."
    fi
}

# shellcheck disable=SC2155
function run_tests() {
    export HOSTS="$(echo "$3" | tr '\n' ' ')"
    shift 3 # Drop the first 3 arguments, the remainder being, optionally, the list of tests to run.
    "$DIR/setup.sh"
    echo
    greenly echo "> Running tests..."
    local begin_tests=$(date +%s)
    set +e # Do not fail this script upon test failure, since we need to shut down the test cluster regardless of success or failure.
    "$DIR/run_all.sh" "$@"
    local status=$?
    echo
    greenly echo "> Tests took $(date -u -d @$(($(date +%s) - begin_tests)) +"%T")."
    return $status
}

function main() {
    begin=$(date +%s)
    print_vars
    verify_dependencies

    if [ -z "$ONLY_DESTROY" ]; then
        provision on "$PROVIDER"
        if [ $? -ne 0 ]; then
            echo >&2 "> Failed to provision test host(s)."
            exit 1
        fi

        if [ -z "$SKIP_CONFIG" ]; then
            configure "$ssh_user" "$ssh_hosts" "${ssh_port:-22}" "$ssh_id_file"
            if [ $? -ne 0 ]; then
                echo >&2 "Failed to configure test host(s)."
                exit 1
            fi
        fi

        run_tests "$ssh_user" "$ssh_id_file" "$ssh_hosts" "$TESTS"
        status=$?
    fi
    if [ -z "$SKIP_DESTROY" ]; then
        provision off "$PROVIDER"
    fi

    echo
    greenly echo "> Build took $(date -u -d @$(($(date +%s) - begin)) +"%T")."
    exit $status
}

main
