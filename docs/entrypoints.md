# Entrypoints

This document describes the various ways that weave as a binary gets launched, and where and how it processes options.

The goal is to enable maintainers to modify functionality without having to reverse engineer the code to find all of the places you can start a weave binary and what it depends upon.

## Components
### weaver

`weaver` is the primary binary: it sets up the weave bridge to which containers are attached, manages IP address allocation, serves DNS requests and does many more things.

Since version 2.0, `weaver` also bundles the Docker Plugin (legacy version) and Docker API Proxy which used to be shipped as separate containers.

### weaveutil
weaveutil is a binary which provides a number of functions for managing an existing weave network. It gets information about the network, attaches and detaches containers, etc.

For the majority of interactions with an existing weave network, you will launch `weaveutil` in some manner or another.

Almost all options can be passed as `--option` to `weaveutil`. These options are created in `prog/weaveutil/main.go` with a table mapping each command to a dedicated golang function.

In addition to operating in normal mode, `weaveutil` has several additional operating modes. Before processing any commands, `weaveutil` checks the filename with which it was called.

* If it was `weave-ipam` it delegates responsibility to the `cniIPAM()` function in `prog/weaveutil/cni.go`, which, in turn, calls the standard CNI plugin function `cni.PluginMain()`, passing it weave's IPAM implementation from `plugin/ipam`.
* If it was `weave-net` it delegates responsibility to the `cniNet()` function in `prog/weaveutil/cni.go`, which, in turn, calls the standard CNI plugin function `cni.PluginMain()`, passing it weave's net plugin implementation from `plugin/net`.

Finally, `weaveutil` is called by `weaver` when it needs to operate in a different network namespace.
Go programs cannot safely switch namespaces; see this [issue](http://github.com/vishvananda/netns/issues/17).
The call chain is `weaver`->`nsenter`->`weaveutil`.

### weave
Wrapping `weaver` and `weaveutil` is `weave`, a `sh` script that provides help information and calls `weaveutil` as relevant.

### weave-kube
weave-kube is an image with the weave binaries and a wrapper script installed. It is responsible for:

1. Setting up the weave network
2. Connecting to peers
3. Copy the weave CNI-compatible binary plugin `weaveutil` to `/opt/cni/bin/weave-net` and weave config in `/etc/cni/net.d/10-weave.conflist`
4. Running the weave network policy controller (weave-npc) to implement kubernetes' `NetworkPolicy`

Once installation is complete, each network-relevant change in a container leads `kubelet` to:

1. Set certain environment variables to configure the CNI plugin, mostly related to the container ID
2. Launch `/opt/cni/bin/weave-net`
3. Pass the CNI config - the contents of `/etc/cni/net.d/10-weave.conflist` to `weave-net`

Thus, when each container is changed, and `kubelet` calls weave as a CNI plugin, it really just is launching `weaveutil` as `weave-net`.

The installation and setup of all of the above - and therefore the entrypoint to the weave-kube image - is the script `prog/weave-kube/launch.sh`. `launch.sh` does the following:

1. Read configuration variables from the environment. When the documentation for `weave-kube` describes configuring the weave network by changing the environment variables in the daemonset in the `.yml` file, `launch.sh` reads these environment variables.
2. Set up the config file at `/etc/cni/net.d/10-weave.conflist`
3. Run the `weave` initialization script
4. Run `weaver` with the correct configuration passed as command-line options


## Adding Options
To add new options to how weave should run with each invocation, you would do the following:

1. determine to which `weave` command you want to add the option(s). `weave` normally is launched as `weave <command> <options> ...`.
2. Add the option to `weave` script help for each `weave` command you wish to make it available. As of this writing, all of the commands and their options are listed in the function `usage_no_exist()`.
3. Add the option to `weave` script option processing for the `weave` command, under the `case $COMMAND in` in the main `weave` script.
4. In the function for the `weave` command, determine if the command should be passed on to `weaveutil` or `weaver`. Pass the option on to `weaveutil` or `weaver`, as appropriate, in the format `--option`, in the function for the `weave` command.
5. Add a command-line option `--option` to `weaveutil` or `weaver` as appropriate.
6. If the option can be configured for CNI:
    * have `prog/weave-kube/launch.sh` read it as an environment variable and set inside
    * set a default for the environment variable in `prog/weave-kube/weave-daemonset-k8s-1.6.yaml` and `weave-daemonset.yaml`
    * if it should be set via `weaver` globally on its one-time initialization invocation, pass it on in `launch.sh`
    * if it needs to be set via `weaveutil` on each invocation of `weave-net`, have `launch.sh` save it as an option in the CNI config file `/etc/cni/net.d/10-weave.conflist` and then have the CNI code in `plugin/net/cni.go` in `weaveutil` read it and use where appropriate
7. Document it!
