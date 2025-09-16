## Release 2.9.0

This release has upgraded dependencies based on CVE recommendations.

Also, from this release onwards, all weave images will use the `iptables-nft` backend for `iptables` by default. To use the legacy mode, set the environment variable `IPTABLES_BACKEND` to `legacy`. To do this for the Docker plugin, export the variable before using the `weave` script. For the Kubernetes addon, set the variable in the manifest.

## Release 2.8.10

This release bumps the Go version to 1.23.1 and the alpine base image to version 3.20.3 as per CVE recommendations.

## Release 2.8.9

This release bumps the Go version to 1.22.2, the alpine base image to version 3.20.2, and upgrades a dependency as per CVE recommendations.

## Release 2.8.8

The alpine base image used by `weave-kube` and `weave-npc` has been upgraded to version 3.20.0 in this release. This removes several CVEs.

## Release 2.8.7

This release has upgraded dependencies based on CVE recommendations.

Also, this release does not provides images for the `ppc64le` and `s390x` architectures, because a crucial upstream dependency (libpcap-0.8, from the debian buster respository) does not provide binaries for those architectures any more.

## Release 2.8.6

From this release onwards, the Docker V2 Plugin will be published for all supported architectures. It will be named using the pattern `rajchaudhuri/net-plugin:<version>-<arch>`. The latest version will also be published as `rajchaudhuri/net-plugin:latest_release-<arch>`. Supported architectures are `amd64`,`arm`,`arm64`,`ppc64le` and `s390x`. It can be installed on a Swarm-mode-enabled Docker host as follows:

```bash
ARCH=$(docker version -f '{{.Server.Arch}}')
docker plugin install rajchaudhuri/net-plugin:latest_release-$ARCH
```

## Release 2.8.5

This release has upgraded dependencies based on CVE recommendations.

* Build with Go version 1.21.9
* A dependency has been updated

## Release 2.8.4

The alpine base image used by `weave-kube` and `weave-npc` was upgraded to version 3.19.1 in the previous release. In this version, the default iptables backend is nftables, and the legacy backend is not included. Our scripts and programs assume legacy as the default backend, and change to nft if autodetected, or if we ask for it. So, the build Dockerfile (reweave/build/Dockerfile) was changed to also install the Alpine `iptables-legacy` package , and change the `iptables-{save,restore}` symbolic links to point to the legacy backend by default.

The `weave-kube` and `weave-npc` images can now log traces if the environment WEAVE_DEBUG is set in the manifest.

## Release 2.8.3

The docker API client version, used by the proxy package and the weaveutil command, was bumped from 1.18 to 1.24. As of March 2024, Docker API versions below 1.24 are deprecated. This means that the minimum supported Docker version is now 1.12.0.

Provision was made in weaveutil program and the weave script to override the API version used, via the environment variable `DOCKER_API_VERSION`. The same variable is used by standard docker clients.

## Release 2.8.2

This is the first release since forking from weaveworks/weave. While no new functionality has been added, there have been many changes.

* Build with Go version 1.21.6
* Dependencies, especially ones with associated CVEs, have been updated
* libCNI has been upgraded to v1.1.2
* A new, Docker-only build process has been created, which produces proper multi-arch images
* A vulnerability scanning process has been created using [grype](https://github.com/anchore/grype)

## Release 2.8.1

Fixes a problem introduced in 2.8.0 for machines whose ID is in /etc/machine-id #3886
Many thanks to contributor @avestuk for this fix.

Also move Kubernetes API calls out of Weave Net daemon #3885
- this reduces the size of the 'weaver' binary and stops it crashing when run on 32-bit ARM.

## Release 2.8.0

This release makes some important changes to trim the "attack surface" of the
Kubernetes install, and improves a couple of reported issues.

* Workaround to fix ipset conflict with iptables #3851, #3882
* Kubernetes: move kernel and CNI setup to init container #3880
* For K8s, stop running in host PID namespace #3876
* NetworkPolicy: avoid logging dropped packets that were not actually dropped #3852
* Build with Go version 1.15.6 #3883

Many thanks to contributors @drigz, @KevDBG and @NeonSludge.

[Full list of changes](https://github.com/weaveworks/weave/milestone/87?closed=1)

## Release 2.7.0

This release improves resiliency in a number of areas,
and extends the Prometheus metrics exported by Weave Net.

Change in behaviour: on Kubernetes, the client source IP is preserved 
when calling from a pod to a service.
This feature, introduced in version 2.4.0 and previously turned on by 
setting `NO_MASQ_LOCAL=1` is now on by default. #3389, #3756

### Features

* Reload router iptables rules if they get cleared, e.g. when firewalld restarts. #3802
  (weave-npc rules are not reloaded)
* Add new `type` and `encryption` labels to `weave_connections` metric #3788, #3789
* Weave Net now exports Go metrics for heap size, garbage collection, etc. #3838
* Register container name and its network aliases with weaveDNS #3084, #3090
* Make DNS listen address configurable #1770, #3231

### Bug fixes

* weave-npc could crash if you deleted a Kubernetes Namespace containing pods #3833, #3836
* Ensure that weave-npc exits and restarts if it crashes #3764, #3792, #3841
* Avoid weave-kube failing on startup due to iptables lock #3828, #3835

### Build and test

* Reduce size of containers (weaveworks/weave goes from 99MB to 83MB) #3624, #3726
* Weave Net is now built with Go 1.14.4, which should improve performance #3838
* CI tests are now run against Docker 19.03.1, Kubernetes 1.14.0 #3687

Many thanks to contributors @berlic, @gobomb, @hairyhenderson, @naemono, @nesc58

[Full list of changes](https://github.com/weaveworks/weave/milestone/81?closed=1)


## Release 2.6.5

Fixes a bug that would leak memory every time a fast-datapath connection
was stopped. #3808
Also avoid a crash when the machine has ipv6 disabled. #3815


## Release 2.6.4

Improves the iptables rule added in 2.6.3 to block just the Weave Net
control port and avoid blocking other uses of 127.0.0.1.	#3811


## Release 2.6.3

This release has a couple of security improvements, and some other fixes.

* Block non-local traffic to the Weave control port	#3805
* Tell Linux not to accept router advisory messages	#3801
* Network Policy Controller: add a metric to show errors while operating	#3804
* Network Policy Controller: don't treat named port as a fatal error	#3790

## Release 2.6.2

This release fixes a regression found in 2.6.1 release and removes a possible case
of CPU spinning when pcap is used in sleeve mode

### Bug fixes

* Weave Net can not be used in fastdp mode and always falls back to sleeve mode #3781, #3783
* Restrict timeout value passed to pcap library to a value less than 2^31 microseconds to
  prevent CPU spinning #3782

## Release 2.6.1

This release fixes a bug discovered since the release of Weave Net 2.6.0 and
an improvement to support iptables 1.8

### Bug fixes

* Weave Net on node restart unable to form connections to peers and connections get stuck in
  pending state and stops responding to API endpoint #3762 #3763

### Other improvements

* Weave Net Kubernetes images by default uses iptables 1.8 with legacy (netfilter) backend with
  option to choose netfilter as backend #3465, #3747


## Release 2.6.0

This release reduces CPU and memory usage in larger clusters, by
sending notifications to a smaller set of peers and coalescing updates
to reduce topology recalculation. #3715, #3732

The default soft limit on connections has been raised from 100 to 200.

### Bug fixes

* Fix a race condition in Kubernetes addon when reclaiming IP
  addresses after node deletion #3724, #3716
* Buffer events so Docker won't drop them, and Weave Net can clean up
  after dead containers #3432, #3705
* Weave reconnect occasionally fails after network interface
  disconnect #3666, #3669, #3676
* Ingress NetworkPolicy would accepts all traffic when specifying both
  IPBlock and port #3653, #3654

### Kubernetes improvements

* Support both podSelector and namespaceSelector in NetworkPolicy #3312, #3647
* Only add default-drop egress rule if network policies are in use #3639
* Manifests use 'apps/v1' rather than deprecated 'apps/v1beta1' #3660
* Avoid Weave Net pods being evicted by setting priorityClassName: system-node-critical #3697
* Manifests use recommended DNS policy ClusterFirstWithHostNet #3692
* Weave Net now tolerates 'NoExecute' taint #3655
* Allow extra arguments to NetworkPolicy controller to be set in an environment variable #3683
* Stop reporting a failure to connect to self #3454, #3585
* Minor reduction in log noise when reclaiming IPs #3710

### Other improvements

* Avoid isolating nodes which have restarted by automatically
  repairing inconsistencies in IP allocation data #1962, #3637, #3708
* Build Weave Net for the s390x architecture #3685
* When a IP address is requested that may be in use, make several
  attempts to claim it before returning an error #3725
* Improve logging for IP allocation updates #3627, #3630
* Improve 'expecting PMTU update' log message on initial connection #3603

### Build and test

* Shut down Kubernetes on node when testing node deletion #3716
* Update the base 'Alpine' container image to version 3.8 #3701
* Update Go to version 1.13.3 #3712
* Update gopacket library #3590
* Pin busybox version to 1.28 to avoid CI failure in Python test #3689
* Remove obsolete weave-daemonset.yaml file #3674

Thanks to contributors @christian-2, @hpdvanwyk, @guirish, @kitt1987,
@mmerrill3, @Pensu, @scritchley, @sidharthsurana, @tanishq-dubey

[Full list of changes](https://github.com/weaveworks/weave/milestone/78?closed=1)


## Release 2.5.2

This release fixes several bugs causing inconsistencies in IPAM and fixes a 
panic in daemon that reclaims and forgets deleted nodes in Kubernetes clusters.

### Bug fixes

* leaking IP addresses in IPAM when weave-net hangs or restarting #3587, #3638
* Panic in kube-utils after config map update error resulting in reclaim daemon
  not performing weave forget on deleted node #3613, #3623
* preventing inconsistent IPAM entries and resolving the conflict in case of
  conflict during IPAM ring merge #3629, #3635, #3632, #3444

[Full list of changes](https://github.com/weaveworks/weave/milestone/79?closed=1)


## Release 2.5.1

This release fixes bugs reported for 2.5 release and small improvements.

### Bug fixes

* prevent warnings in kernal logs due to use of physdev model for
  non-bridged traffic #3449, #3453
* Check and report the error occurred while getting the list of Kubernetes 
  peers #3581, #3582

### Other improvements

* Update Ruby version to match Weaveworks website #3458
* Fix the link to "Troubleshooting the Kubernetes Addon" section in the docs #3461
* Document concept of connection limit in Weave #3395, #3413

[Full list of changes](https://github.com/weaveworks/weave/milestone/77?closed=1)


## Release 2.5.0

This release adds support for Kubernetes `hostPort` mapping (#3016,#3356)
and the `ipBlock` NetworkPolicy feature (#3168,#3367)

### Bug fixes

* Fix a crash at start-up on Docker for Mac #3405, #3408
* Network policy: block ingress traffic when no namespaceSelector or
  podSelector is specified #3347
* Reclaim IP addresses which are locked by a non-existent peer #3386, #3416
* Fix a crash when blank IP data was loaded #3067, #3415

### Other improvements

* If a connection is downgraded to the slower "sleeve" mode, Weave Net
  will now periodically try to upgrade it to "fast datapath" again. #1737, #3385
* Reclaim removed Kubernetes nodes' IP space when they are deleted,
  rather than on next restart #3372, #3399
* Replace Kubernetes livenessProbe with readinessProbe, so the pod is
  not killed if it runs slowly #3471, #3421
* In Kubernetes NetworkPolicy controller, remove the need to maintain
  a set of local pod IP addresses #3344, #3423
* Don't crash on Kubernetes named port in NetworkPolicy, just report as unsupported #3375
* Ensure the `weave` network bridge is accessible on Linux kernels
  older than 3.14 #3442, #3297, #3239
* Better reporting in the logs if the `weave` network device is in the Down state #3133, #3381
* Change log-level to debug of calls through the Docker proxy, to reduce noise #3439
* Add `--without-masquerade` option to `weave expose`, so external
  services can see the original container IP address #3388
* Include Kubernetes cluster information in checkpoint call #3324,#3431
* Bump go-odp dependency, so that `fastdp` works on the 4.19 kernel #3430

### Build and Testing

* CI builds on master branch now publish images for all platforms
* Fix golint path and use https for download of libpcap #3435
* Update Kubernetes client-go to v8.0.0, removing code licenced under LGPL3 #3358,#3366
* Migrate CircleCI to V2, which is much faster #3255,#3270

### External Contributors

Thanks to the following contributors:

* @Ashiroq
* @leprechau
* @lkpdn

[Full list of changes](https://github.com/weaveworks/weave/milestone/74?closed=1)


## Release 2.4.1

This release fixes several bugs causing inconsistencies in IPAM for
Kubernetes users whose clusters scale up and down over time.

### Bug fixes

* Nodes unable to connect after Kubernetes addon erroneously reclaimed
  node without any IP addresses #3392, #3393
* Kubernetes addon could have run out of free IP addresses after nodes
  are deleted #3384, #3400
* Kubernetes addon had reduced free IP addresses due to not reclaiming
  IP addresses when node name is re-used #3397

### Other improvements

* Support `--label` in `WEAVE_DOCKER_ARGS` when starting Weave #3370,#3371
* Add missing `--token` argument in help for `weave launch` #3226, #3379
* Print defunct processes after smoke-tests #3362

[Full list of changes](https://github.com/weaveworks/weave/milestone/75?closed=1)


## Release 2.4.0

This release introduces a support for Kubernetes Egress Network Policy (#2624, #3313)
and adds a mechanism for preserving the client source IP address to enable
`externalTrafficPolicy: Local` on Kubernetes (#2924, #3298).

### Bug fixes

* Increase the ipset list size which prevents weave-npc from crashing on older
  kernels when more than eight Kubernetes Namespaces are used (#3289, #3305).
* Avoid a possible livelock when reclaiming IP address space in weave-kube (#3317).
* Ensure `xtables.lock` is mounted as a file so that kube-proxy can take the lock
  if it has started after Weave Net (#3351, #3353).
* Upgrade the CNI plugin symlinks only if the plugin has changed (#3337, #3345).

### Other improvements

* Manipulate the Kubernetes node status `NetworkUnavailable` so that Pods can be
  scheduled on nodes when the GCE cloud provider is in use (#3249, #3307, #3332, #3334).
* Refrain from creating a subprocess for configuring a network interface in
  a container network namespace (#3291).
* Protect against handling the CNI plugin request with the host namespace which
  prevents Weave Net from misconfiguring the host network (#3206, #3346).
* Weave Net can be run on minikube VM (#3124).
* Add `org.opencontainers.image.*` labels to Dockerfiles to improve association
  of the container images with git revisions (#3299).
* Improve the error message when running `weave reset` on Kubernetes (#3319).

### Build and Testing

* Use `dep` instead of `git submodules` for managing external packages (#3268).
* Fix usage of `manifest-tool` in Makefile (#3320).
* Update Kubernetes to 1.11 for the integration tests (#3340).

### External Contributors

Thanks to the following contributors:

* @kitt1987
* @louismunro
* @Nodraak
* @stevenjohnstone

[Full list of changes](https://github.com/weaveworks/weave/milestone/73?closed=1)

## Release 2.3.0

### Security fixes

* By default, do not expose Weave "/status" and "/report" to all (0.0.0.0) when
  running on Kubernetes #3271

### Other improvements

* Increase the default connection limit for Weave peers (from 30 to 100) when
running on Kubernetes, so that more peers could directly connect #3265

## Build and test

* Build Weave Net with Go 1.10.1 #3273
* Run integration tests against Kubernetes 1.10.0 #3266

[Full list of changes](https://github.com/weaveworks/weave/milestone/70?closed=1)

## Release 2.2.1

### Bug fixes

* Fix a bug in weave-npc which would allow ingress traffic to Kubernetes Pods selected
  by a NetworkPolicy in which source and destination selectors were the same #3222,#3237
* Fix a bug in weave-npc which would crash if a previously deleted Kubernetes Namespace
  has been created again #3247,#3250

### Other improvements

* Increase the default connection limit for Weave peers (from 30 to 100), so that
  more peers could directly connect #3234
* When doing a rolling update of Weave Net on Kubernetes, allow each node five seconds
  to initialize before rolling next Weave Net Pod, so that issues at startup will halt
  the rollout and not spread across the whole cluster #3235
* Install common CA certificates from Alpine Linux package instead of copying
  them manually #3236

### External contributors

Thanks to the following contributors:

* @alok87

[Full list of changes](https://github.com/weaveworks/weave/milestone/71?closed=1)

## Release 2.2.0

This release improves the way Weave Net configures Linux network
devices and network filter rules, so that it is more robust in the face
of unexpected changes in its environment. #3204,#3224

As a consequence of these changes, the `weave attach` command will now
fail unless the Weave Net daemon is up and running - previously it was
possible to run independently as long as you managed all IP addresses
yourself.

## Other improvements

* Update library miekg/dns for CVE-2017-15133 (details under embargo) #3223,#3227
* Reduce the volume of logging from weave-npc #3183
* Add ability to set log level for Docker "v2" plugin, and change
  default log level from DEBUG to INFO #3197
* Downgrade log messages about Discovery and Expiration to DEBUG level #3202,#3203
* Use command-line parameter for WeaveDNS address in Docker proxy #3196

## Bug fixes

* Ensure that rules to block traffic for NetworkPolicy are placed
  ahead of rules that Kubernetes has added to allow other traffic #3209,#3210

## Build and test

* Update CI tests to use Kubernetes 1.9.2 #3229
* Remove "daily update" from test VMs that only run for a few minutes #3224

## External Contributors

Thanks to the following contributors:
@vetal4444

[Full list of changes](https://github.com/weaveworks/weave/milestone/67?closed=1)


## Release 2.1.3

This release fixes a race-condition in the IP reclaim code for weave-kube
where two nodes could end up fighting over the same space and break
connectivity #3190, #3192

## Release 2.1.2

This release fixes a couple of bugs discovered since the release of Weave Net 2.1.0

##Bug fixes

* Fix crash seen when starting 10-15 nodes simultaneously #3184,#3186
* Fix NetworkPolicy blocking traffic if updates come out of order from Kubernetes #3177,#3181

Thanks to the following contributors:
@zignig

[Full list of changes](https://github.com/weaveworks/weave/milestone/66?closed=1)


## Release 2.1.1

As 2.1.0, but fixing a couple of installation glitches. #3175,#3176

## Release 2.1.0

##New Features

Improved Kubernetes Network Policy - Weave Net now supports the
'v1' policies introduced in Kubernetes 1.7 as well as the 'beta'
policies supported previously. See [Kubernetes 1.7 changelog](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG-1.7.md#network)
for differences. To use old policies, `--use-legacy-netpol` argument
should be passed to `weave-npc`. #3105,#3141,#3151,#3169

Weave Net now reclaims IP addresses owned by Kubernetes nodes which
have been deleted from the cluster - this avoids running out of IP
addresses when many nodes are added and deleted over a long period.
#2797,#3149,#3170,#3172

##Other improvements

* Export a Prometheus-style metric giving count of unreachable peers #3119
* Update 'gopacket' library to reduce memory use by approx 15MB #3160
* Replace bundling the 'docker' binary with our own code to avoid
  security vulnerability alerts and save space #2957,#3110

##Bug fixes

* When `weave expose` is used, allow traffic into the Weave network -
  up till version 1.12 Docker would do this for us, but in 1.13 they
  stopped so now we do it. This change makes `weave expose` require
  Weave Net to be running. #2758,#3122
* Arm64 build now works on non-kubernetes installs #2832,#3110
* TX offload was being disabled in 'awsvpc' mode, which slows down packet sending #3089
* Removed spurious 'nil' in logs from CNI DEL operation #3143

##Build and test

* Images are now also built for the ppc64le platform #3129
* Tweak build scripts to run on OSX as well as Linux #3135

##External Contributors

Thanks to the following contributors:
@caarlos0
@dtshepherd

[Full list of changes](https://github.com/weaveworks/weave/milestone/59?closed=1)


## Release 2.0.5

Bug fixes

* Fix /etc/hosts inside containers so the container's name resolves to
  its Weave Net address #3136,#3138
* New weave-kube config for Kubernetes 1.7 and 1.8 which resolves an
  intermittent conflict with kube-proxy that could break Weave Net
  until reboot #2998,#3134
* Remove persistence file in `weave reset` when using Docker plugin V2 #3103,#3114

Build and test

* Some trivial changes to placate go-lint #3137

[Full list of changes](https://github.com/weaveworks/weave/milestone/63?closed=1).


## Release 2.0.4

Bug fixes

* weave-npc failed on Centos 7, due to older 'ipset' version in kernel #3099,#3100

[Full list of changes](https://github.com/weaveworks/weave/milestone/62?closed=1).


## Release 2.0.3

Bug fixes

* Weave-npc would crash on a policy with no 'from' part - regression introduced in 2.0.2 #3095,#3096,#3097

[Full list of changes](https://github.com/weaveworks/weave/milestone/61?closed=1).


## Release 2.0.2

Bug fixes and minor improvements

* Fix race condition in weave-npc which would intermittently block all traffic for a namespace  #3057,#3059
* Ensure Fast Datapath works on machines that need to mount the kernel module dynamically #3080
* Regression: weave-npc would block everything if `kubelet --hostname-override` was used #3049,#3051
* Fix netfilter rules to block containers from accessing the Weave Net control endpoint #3093
* If DNS server is off then disable proxy DNS registration, to avoid spurious errors #3054,#3088
* Add comments to each iptables rule and ipset, to help when troubleshooting #3064
* Remove code that checked for an outdated fallback address for Kubernetes api-server #3071
* Add a label to the weavedb image so it can be filtered out by tools #3066
* Fix various build and continuous-integration failures #3061
* Print 'help' text faster in the weave script #3056
* Add an option to create continuous integration hosts in different ways  #3060
* Remove remnants of the pre-2.0 proxy and plugin from build and test #3035,#3036

[Full list of changes](https://github.com/weaveworks/weave/milestone/60?closed=1).


## Release 2.0.1

Bug fixes and minor improvements

* Fall back to slower data path (`sleeve`), rather than crashing, when the machine lacks VXLAN support (required for “fast data path”, `fastdp`)  #3043
* Fix bug in processing of arguments when Docker has TLS enabled, rather than crashing with invalid peers list, e.g. `lookup --tlsverify: no such host` #3039
* Add `kube-system` namespace back to `weave-kube`'s YAMLs, preventing omissions leading to errors like `error contacting APIServer: the server does not allow access to the requested resource` #3033,#3042
* Fix release script to prevent ARM64 binaries to end up in AMD64 `net-plugin`, leading to `Error response from daemon: dial unix /run/docker/plugins/<id>/weave.sock: connect: no such file or directory` when installing `net-plugin` #3045
* `weave reset` and `weave rmpeer` now only contact Weave Cloud when Weave Net is configured with a Weave Cloud token, preventing unnecessary requests and potentially confusing `401 Unauthorized` errors in Weave Net’s logs #3044

[Full list of changes](https://github.com/weaveworks/weave/milestone/58?closed=1).


## Release 2.0.0

New Features

Peer Discovery via Weave Cloud

You can now get all your Weave Net peers to find each other via the
Weave Cloud service, instead of maintaining a list of peers at
startup. #2799,#2827

See the [docs
page](https://github.com/weaveworks/weave/blob/master/site/using-weave/weave-cloud.md)
for more details

New Docker Plugin

Docker has a [new plugin
system](https://docs.docker.com/engine/extend/) which improves the
installation UX and solves some issues around startup.  This means
Weave Net 2.0 can now run with Docker in "swarm mode" and supports the
`docker service` command. #2396,#2397,#2651,#2727,#2805,#2816,#2905,
#2906,#2929,#2932,#2945,#2950,#2956,#2963,#2964,#2966,#3019

The previous Docker Plugin is still available and can be installed as before.

All of Weave Net now runs in one container

Previously we had three separate containers for routing, Docker API
proxy and Docker plugin. Running everything in one simplifies start-up
and removes the need to detect various error
conditions. #1642,#2897,#2936,#2945,#2946,#2951,#2960

The individual commands ‘weave launch-router’, ‘weave launch-plugin’,
etc., have been removed. You can turn off the plugin and proxy with
new command-line options. In keeping with Semantic Versioning, we have
changed the major version number for this release.

Other new features

* Kubernetes configuration now comes from our “Launch Generator” that
  allows different options to be selected via
  URL. #2754,#2903,#3000,#3001
* `weave-kube` now stores data about IP allocation in `/var/lib/weave`
  on the host instead of in a Kubernetes volume.  This means that the
  data will persist across pod deletion and re-creation, e.g. during
  an upgrade of Weave Net, which makes restarts more
  reliable. #2610,#2967
* `weave-kube` turned on rolling updates, so careful manual handling
  of updates is no longer required. #3024

Bug fixes

* Kubernetes Network Policies which allowed a specific set of pods to
  connect would block all pods on other hosts. Revert the change in
  v1.9.6 which ignored pods on other hosts #3025,#3028

Features removed

* `weave run` has been removed.  This was the original method provided
  to start containers with Weave Net, but it always required care over
  timing of start-up, and we now provide three alternative, better,
  ways. You can replicate the effect by calling `docker run` then
  `weave attach`.  Similarly `weave start` and `weave restart` were
  removed. #2353,#2885
* Everything deprecated more than one release ago has been removed, so
  if you use it now you get an error rather than a warning. This
  includes the ‘create-bridge’ command and older command-line
  arguments, e.g. `--iprange` was replaced by `--ipalloc-range`
  #2901,#2909,#2913,#2942,#2989,#2991

Functions moved from shell-script to Go code.

This enables more precise error-checking and runs a bit faster. It has
also enabled us to shrink the size of images downloaded: `weave-kube`
is 101MB compared to 163MB previously #2953,#2954,#2974

Specific items that moved from shell-script to Go:
* Setting up the `weave` bridge #1958,#2975,#2977,#2978
* Container attachment #2947
* Creation of the ’weave’ default plugin network #2920

Minor improvements

* You can now restart the Weave Net router without requiring the proxy to be enabled #2112
* Plugin (legacy version) now respects `--ipalloc-default-subnet` option #2919
* The `weave` script now detects and issues an error message if
  `weave-kube` is running and you attempt to launch again from the script. #2709/#2966
* It is now possible to choose the the MAC address of the `weave`
  bridge using `--name`, in case your hosts have identical unique
  IDs. #2900
* Relaxed Kubernetes tolerations for Weave Net's daemonset in order to
  match any node (previously, only taints directed at master). #3018
* Kubernetes' `seLinuxOptions` configuration is now empty by default,
  to reduce spurious failures on hosts not using seLinux. #3001
* Improved reliability of namespace changes via `nsenter`. #2992
* `weave ps` now fetches the list container IDs internally, rather
  than calling out to `docker ps` #2814,#2898
* at startup, actively remove dead containers’ Weave Net IP addresses from IPAM #3013
* at startup, only check live containers to see if they have an
  existing Weave Net IP address #2815,#2829
* Weave Net CNI plugin now logs but does not raise an error if
  anything goes wrong during network interface delete, to be more
  compatible with Kubernetes 1.6. #2928
* Stop running a shell in “privileged” mode when it’s only writing a file #2838
* New internal REST endpoint to return all IP address mappings. #1350
* Changed the wording where we do not log the password #2833
* Fixed typo in plugin error messages #2894

Build and test

* Weave Net is now built with Go version 1.8, which has better code
  generation and garbage collection #2914
* During smoke-tests, use a webserver instead of just `ping` so we get
  a more realistic test that the Weave network is working #2918
* When installing dependencies for the build container, use a
  keyserver port that's better for firewalls #2812
* Kubernetes test script now scales up to more hosts, and works with
  Kubernetes 1.6 #2837,#2853,#2923
* Other minor build improvements and refactoring #2760,#2910

[Full list of changes](https://github.com/weaveworks/weave/milestone/49?closed=1).


## Release 1.9.8

Bug fixes and minor improvements

* Fix weave-npc blocking NodePort and any other non-local access #3011,#3014
* Fix bug where IPAM would duplicate a fixed IP address assigned via Docker plugin #3003,#3010

[Full list of changes](https://github.com/weaveworks/weave/milestone/56?closed=1).


## Release 1.9.7

This is identical to 1.9.6 with one additional bug-fix:

* weave-npc would block everything if `kubelet --hostname-override` was used #2995,#2996

## Release 1.9.6

Bug fixes and minor improvements

* Ensure that Kubernetes pods can contact a service implemented within
  the same pod, by turning on "hairpin mode". This is required because
  of a quiet change between Kubernetes 1.5 and 1.6. #2993
* Network Policy Controller (`weave-npc`) now checks local addresses
  only, so it doesn't interfere with cross-cluster traffic. It should
  be more efficient too #2622,#2973,#2979
* Stop reporting back to Kubernetes any issues encountered when
  deleting a pod's network interface. This is required because
  of a quiet change between Kubernetes 1.5 and 1.6. #2921,#2928
* Fixed an issue whereby `weave-npc` couldn't start because one
  `ipset` was referring to another one and could not be destroyed #2915,#2949
* Improved the code which checks whether the kernel supports `ipset` #2934,#2935
* `weave-npc` now creates ipsets with only valid xml characters in the
  name #2958,#2959

Build and Testing

* In build container use cross-compilers from debian package
  repository, so they match other components #2940
* Pin the version of the linting tool `shfmt` so the set of things it
  checks is stable #2987
* Fix lint error in script that runs smoke-tests #2962
* Moved website publishing from Wordpress to Netlify #2986

[Full list of changes](https://github.com/weaveworks/weave/milestone/55?closed=1).

## Release 1.9.5

Bug fixes and minor improvements

* Improve log messages generated if "hairpin" conditions are detected,
  to make clear which kind is likely to cause problems #2808/#2926
* Filter out IPv6 peer addresses from Kubernetes; Weave Net currently
  only supports IPv4 #2904/#2912
* Fix rare crash during initialization of weave-kube #2893/#2892
* Include overlay and encryption modes in checkpoint reports, in case
  this is relevant to a version upgrade #2771/#2907

Build and Testing

* Ensure CI build can run gcloud tools  #2887
* Prevent kubeadm from upgrading Kubernetes if we are trying to test an older version #2886
* Upgrade build scripts to support Kubernetes 1.6 #2880

[Full list of changes](https://github.com/weaveworks/weave/milestone/54?closed=1).

## Release 1.9.4

Bug fixes and minor improvements

* Support Kubernetes 1.6 by creating a new DaemonSet #2777,#2801
* Support Kubernetes 1.6 by allowing CNI callers to send a
  network-delete request for a container that is not running or has
  never been attached to the network #2850
* Leave non-weave ipsets alone in Network Policy Controller (e.g. when
  running Weave Net alonside keepalived-vip) #2751,#2846
* Fix various small issues revealed by 'staticcheck' tool #2843,#2857
* Avoid leaving 'defunct' processes when weave-kube container restarts #2836,#2845
* When using the CNI plugin with a non-standard network configuration
  file, the weave bridge could get the same IP as a container, if
  'weave expose' hadn't run at that point #2839,#2856

Build and Testing

* Check that no defunct processes remain after each test #2852
* Update build and test scripts to work with Kubernetes 1.6 beta #2851

[Full list of changes](https://github.com/weaveworks/weave/milestone/53?closed=1).

## Release 1.9.3

Bug fixes and minor improvements

* Fixed a race condition in Fast Datapath encrypted connections which could lead Weave Net to crash: #2824, #2825.

## Release 1.9.2

Bug fixes and minor improvements

* Fix a weave-kube bug when `br_netfilter` or `xt_set` module is compiled into
  kernel #2820/#2821
* Detect the absence of the required `xt_set` kernel module #2821

## Release 1.9.1

Bug fixes and minor improvements

* Fix a race condition when the Weave Net container is restarted
  which could allow a new container to be allocated the same IP
  address as an existing one #2784,#2787
* Handle the message type received when a pod has been deleted during
  Kubernetes api-server fail-over #2772,#2773
* Make weave-kube work with `dockerd --iptables=false` #2726
* Ensure we have the right kernel modules loaded for Network Policy in weave-kube #2819
* Reference-count addresses in Network Policy Controller, to avoid
  errors when updates come in an unexpected order #2792,#2795
* Allow the soft connection limit to be raised in weave-kube, so
  larger clusters can be created #2781
* WeaveDNS was incorrectly case-sensitive for reverse DNS lookups #2817,#2818

Build and Testing

* Scripts to create VMs to run automated tests were rewritten to use
  Terraform and Ansible, to make it much easier to test with different
  versions of components such as Docker and Kubernetes #2647,#2694,#2775,#2796
* Upgrade to latest Weaveworks common build-tools #2780
* Improve encryption tests #2793
* Update vishvananda/netlink library to bring in changes we had previously forked #2790
* Slight change to the build container to avoid permission errors and slow builds #2761,#2802

## Release 1.9.0

Highlights:

* Encryption is now available for Fast Datapath connections, which
  greatly improves the performance. #1644,#2687
* We now build images for Intel/AMD 64-bit, ARM and ARM 64-bit. #2713
* Weave Net Docker images are now labelled with description, vendor, etc. #2712
* `weave status connections` now shows the MTU #2389,#2663
* CNI plugin is now a stand-alone binary that does not depend on Docker #2594,#2662
* Embedded docker client updated to version 1.10.3 #2395

**NOTE:** The move to multi-architecture required that we update the
  embedded Docker client, and this has the effect that this release of
  Weave Net will not work with Docker installations older than
  1.10.

[Full list of changes](https://github.com/weaveworks/weave/issues?q=milestone%3A1.9.0).

## Release 1.8.2

Bug fixes and minor improvements

* Fixed a bug where looping flows were installed which caused high CPU
  usage #2650, #2674
* Fixed a bug where Kubernetes master could not contact pods #2673, #2683
* Fixed a bug where weave-kube was crashing in a loop due to invalid
  Weave bridge state #2657
* Fixed a bug where iptables NAT rules were not appended due to
  "temporary unavailable" iptables error #2679
* Added a detection of enabling the hairpin mode on the Weave bridge port
  which caused installation of looping flows #2674
* Added a detection of overlaps between Weave and the host IP address
  ranges when launching weave-kube #2669, #2672
* Added logging of connections blocked by weave-npc #2546, #2573

More details in the [change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.8.2).

## Release 1.8.1

Bug fixes and minor improvements

* Fixed weave-npc crash from Succeeded/Failed pods #2632,#2658
* Fixed occasional failure to create Weave bridge on node reboot
  #2617,#2637
* Fixed a bug where weave-kube would fail to install when run with
  unrelease snapshot builds #2642
* Improved conformance to CNI spec by not releasing IP addresses when
  a container dies #2643
* Improved troubleshooting of install failure by creating CNI config
  after Weave Net is up #2570
* "up to date" shown even when the version check was blocked by
  firewall #2537,#2565,#2645
* "Unable to claim" message on re-launching Weave after using CNI
  #2548,#2577
* Eliminated spurious IP reclaim operations when IPAM was disabled
  #2567,#2644
* Include `jq` tool in our build VM configuration #2656

## Release 1.8.0

Highlights:

* Exposed network policy controller Prometheus metrics
  weaveworks/weave-npc#23, #2595, #2549
* Exposed router Prometheus metrics #2535, #2547, #2523, #2579, #2578,
  #2568, #2560, #2561

More details in the [change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.8.0).

## Release 1.7.2

Bug fixes and minor improvements

* Fixed an error where the Docker plugin could fail to attach a
  container with a `bridge "weave" not present` error #2540/#2541
* Fixed `cannot connect to itself` panic on weave launch #2527/#2543
* Fixed inferred initial peer count when target peers includes self
  #2481/#2543
* Fixed compilation on Raspberry Pi #2506/#2538

## Release 1.7.1

Bug fixes and minor improvements

* Added utility to recover container addresses without asking Docker #2531
* Improved CI #2274

## Release 1.7.0

Highlights:

* weave-kube - Deploy Weave Net to Kubernetes with a single command
* weave-npc - Kubernetes network policy controller implementation

More details in the [change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.7.0).

## Release 1.6.2

Bug fixes and minor improvements

* Fixed hang after stopping and restarting on Docker 1.12 #2469/#2502
* Avoid an error on Google container images by checking for tx offload support #2504
* Fixed an issue where the supplied peer list could be ignored when restarting after failure #2503/#2509
* Check for empty peer name on launch #2495/#2501

More details in the [change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.6.2).

## Release 1.6.1

Bug fixes and minor improvements

* `weave ps` was occasionally failing to list allocated addresses of containers #2388/#2418/#2445
* `weave launch[-router]` on 4.2 kernel would appear to succeed even if the fast datapath VXLAN UDP port was in use by a different process #2375/#2474
* Launching the proxy would fail when the Docker daemon could not be detected #2457/#2424
* The CNI plugin did not work with Apache Mesos #2394/#2442
* Router stopped working after a restart in the AWSVPC mode #2381/#2409
* Router crashed when the Docker API endpoint parameter was explicitly set to empty #2421/#2467
* The CNI plugin did not work on recent versions of Docker for Mac #2434/#2442
* The CNI plugin assigns an IP to the bridge if necessary, which avoids failures if `weave expose` has not run yet #2471
* Distinguish peer name collisions from attempts to connect to self in logs #2460
* Improve host clock skew detection message #2174
* Improve the error message returned when executing `weave launch-plugin` without the router running #2293/#2416
* The `create-bridge` subcommand was not enabled in the fast datapath mode #2464/2466
* Allow users to omit `weave setup[-cni]` by initializing the CNI plugin on the `launch[-router]` subcommand #2435/#2442
* Reduce verbosity of fast datapath miss event logs #1852/#2417
* Include the `ipam` option in the help output of the `status` subcommand #2425/#2426
* Remove a harmless duplication of the `--no-dns` parameter #2430
* Internal refactoring
* Improvements to testing and building
* Improvements to the documentation

More details in the [change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.6.1).

## Release 1.6.0

## Highlights

* A new [AWS VPC](https://weave.works/docs/net/latest/using-weave/awsvpc/) mode
  that leverages Amazon Virtual Private Cloud for near-native network
  performance, as an alternative to the Sleeve and Fast Datapath
  overlays
* Docker 1.12 introduced some internal changes that made it
  incompatible with previous version of Weave Net - this version
  restores compatibility
* An [operational guide](https://weave.works/docs/net/latest/operational-guide)
  detailing best practices for deploying and operating Weave Net
* Changes to the target peer list are remembered across restarts,
  making it much easier to deploy resilient networks
* The version checkpoint now transmits network mode (e.g. 'awsvpc')
  and kernel/docker versions to us to inform and guide our development
  efforts. See the [installation documentation](https://weave.works/docs/net/latest/installing-weave/)
  for instructions on disabling the checkpoint feature.

More details in the [change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.6.0).

## Release 1.5.2

Bug fixes and minor improvements

* Weave Proxy did not flush the initial http header in the Docker event stream, which could cause Docker Swarm to show all nodes pending. #2306/#2311
* When using the CNI plugin, if a container was removed and quickly replaced by another using the same IP address, other containers might be unable to contact it. Send an address resolution protocol message to update them. #2313
* Avoid Docker hanging for 1 minute in `weave launch` if the plugin had not shut down cleanly #2286/#2292
* Print an error message when Weave bridge mode is changed without `weave reset` #2304
* Eliminate spurious warning message from IP allocator on plugin shutdown #2300/#2319
* Display error message when address requested in a subnet that is too small (/31 or /32) #2282/#2321
* Add short wait after `weave reset` to allow updates to reach peers #2280
* Weave was occasionally unable to claim existing IP address immediately after launch #2275/#2281
* Refactor some integration tests to run faster and more reliably #2291

## Release 1.5.1

Bug fixes and minor improvements

* Persisted data that was rendered invalid by changing peer name or
  allocation range is detected and removed automatically, preventing
  crashes and hangs #2246/#2209/#2249
* `weave rmpeer` persists the range takeover in case the peer on which
  it was executed dies subsequently #2238
* Launching a container with an explicit `WEAVE_CIDR` in the
  allocation range now waits instead of erroring if the allocator
  hasn't finished initialising #2232/#2265
* Weave DNS now responds to AAAA queries with an empty answer section,
  instead of NXDOMAIN which could be cached and block subsequent
  resolution of A records #2244/#2252
* Many improvements to the documentation.

More details in the [change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.5.1).

## Release 1.5.0

## Highlights

- A new [Container Network Interface](https://github.com/appc/cni#cni---the-container-network-interface) plugin.
- This release is much more robust against unscheduled restarts,
  because it persists key data to disk.
- New configuration options that are useful when you create or
  auto-scale larger networks.
- Weave now periodically checks for updates (can be disabled)

Plus many bug fixes and minor enhancements. More details below and in
the [change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.5.0).

## Release 1.4.6

Fixes some issues encountered by our users.

* Restarting a peer could leave stale entries in WeaveDNS
  #1867/#2023/#2081
* Weave proxy occasionally failing to attach any new containers
  #2016/#2049

Other fixes:

* Resolved a crash when a restarting peer re-connected to a peer that
  had not received the latest IPAM data #2083/#2092
* Make IP address space available immediately after a dead peer is
  removed #2068

## Release 1.4.5

Higher performance for multicast and broadcast traffic when using
Weave's Fast Datapath

* The flow rule to deliver broadcast and multicast packets in-kernel
  was not created correctly, hence every such packet caused a
  context-switch to the software router #2003/#2008

Other fixes:

* Remove DNS entries for containers that are being restarted by Docker
  but are not live yet #1977/#1989
* Don't let one failing allocation attempt prevent others from
  succeeding; they could be in different subnets which are more
  available #1996/#2001
* Don't complain on second router launch on kernels that lack support
  for Fast Datapath #1929/#1983
* Fix build broken by change in libnetwork IPAM API #1984/#1985

## Release 1.4.4

Fixing a rather serious issue that slipped through our preparations
for Docker 1.10:

* Restarting Docker or rebooting your machine while the Weave Net
  plugin is running causes Docker 1.10.0 to fail to start #1959/#1963

Also one other small fix:

* Avoid a hang when trying to use plugin and proxy at the same time
  #1956/#1961

## Release 1.4.3

Preparing for Docker 1.10, plus some bug-fixes.

* Avoid hang in Docker v1.10 on `docker volume ls` after `weave stop`,
  `weave stop-plugin` or `weave reset` #1934/#1936
* Fix "unexpected EOF" from Docker 1.10 on `docker exec` with Weave
  proxy `--rewrite-inspect` enabled #1911/#1917
* Avoid losing DNS entries and potentially double-allocating IP
  Addresses allocated via plugin, on router restart; also extend
  `weave ps` to show IP addresses allocated via plugin #1745/#1921
* Stop creating lots of copies of `weavewait` program in Docker
  volumes #1757/#1935
* Prevent container starting prematurely when proxy in
`--no-multicast-route` mode #1942/#1943
* Log error message from plugin rather than crashing when weave not
  running #1906/#1918
* Warn, don't error, if unable to remove plugin network in 'weave
  stop', to avoid breaking the usual upgrade or config change process
  #1900/#1919
* Don't crash if network conditions suggest only very small packets
  will get through #1905/#1926
* Cope with unexpected errors during route traversal when starting
  container via proxy #1909/#1910/#1932

## Release 1.4.2

Bug-fixes and minor improvements.

* A race condition in weavewait that would occasionally hang
  containers at startup #1882/#1884
* Having the plugin auto-restart prevents successful `weave launch` on
  reboot #1869
* Work round weave router failure on CoreOS 4.3 caused by kernel bug
  #1854
* `weave launch` would exit with error code on docker <1.9 #1851
* Running `eval $(weave env)` multiple times would break `eval $(weave
  env --restore)` #1824/#1825
* Don't complain in `weave stop` about "Plugin is not running" when
  plugin is not enabled #1840/#1841
* `weave --local launch` would fail if utility program
  `docker_tls_args` could not be found #1844
* Improved error reporting when TLS arg detection fails #1843
* Improve error reporting when docker isn't running #1845
* Add `--trusted-subnets` usage to `weave` script #1842
* weave run can hang under rare combinations of options #1858

## Release 1.4.1

This is a bug-fix release to cover a few issues that came up since the
release of 1.4.0.

* Weave would fail to launch when `$DOCKER_HOST` was set to a TCP socket secured with TLS #1820/#1822
* Weave would fail to stop when run against Docker pre-1.9 #1815/#1817
* Issue a warning instead of failing on `--with-dns` option to proxy, which was removed #1810/#1812
* Make `weave version` show the plugin version #1797/#1813
* Make `weave launch` show when the a container is restarting #1778/#1814
* Make `weave launch` fail if the plugin is running, for consistency with router and proxy. #1818/#1819

More details in the [change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.4.1).

## Release 1.4.0

**Highlights**

* The Docker Network plugin can now operate without a cluster store, so it is now run by default.
* You can now use the fast datapath over trusted links and Weave encryption over untrusted links, in the same network.

Plus many bug fixes and minor enhancements. More details below and in
the [change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.4.0).

## Release 1.3.1

**Highlights**

* The minimum Docker version has been increased to 1.6 due to the
  [upcoming deprecation of Dockerhub access](https://blog.docker.com/2015/10/docker-hub-deprecation-1-5/)
  for old clients. From December 7th onwards previous versions of the
  `weave` script will fail to pull down images from the hub; if you
  are unable to upgrade to 1.3.1 immediately you can work around this
  by running `weave --local setup` in conjunction with a compatible
  Docker client installation
* Docker networking plugin now works with older kernels and allows you
  to configure the MTU

More details in the
[change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.3.1).

## Release 1.3.0

**Highlights**

This release includes a Docker Plugin, so you have the option to use
Weave Net that way. More details in the
[change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.3.0).

## Release 1.2.1

This release contains a number of bug fixes and minor
enhancements. More details in the
[change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.2.1).

The release is fully compatible with 1.2.0 versions, so existing
clusters can be upgraded incrementally.

## Release 1.2.0

**Highlights**

This release introduces the 
[Fast Data Path](http://docs.weave.works/weave/master/head/features.html#fast-data-path),
which allows Weave networks to operate at near wire-level speeds. This
new feature is enabled by default.

Other highlights:

- auto-configuration of TLS for the Weave Docker API proxy, making it
  easier to run Weave on Macs and in conjunction with tools like
  Docker Swarm
- support for restart policies on application containers and weave
  infrastructure containers
- better compatibility with recent and future Docker versions

More details in the 
[change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.2.0).

## Release 1.1.2

This release contains a small number of bug fixes. More details in the
[change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.1.2).

The release is fully compatible with 1.1.0 versions, so existing
clusters can be upgraded incrementally. When upgrading from 1.0.x,
note the compatibility information in the
[Installation & Upgrading instructions for Weave 1.1.0](https://github.com/weaveworks/weave/releases/tag/v1.1.0).

## Release 1.1.1

This release contains a number of bug fixes and minor
enhancements. More details in the
[change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.1.1).

The release is fully compatible with 1.1.0 versions, so existing
clusters can be upgraded incrementally. When upgrading from 1.0.x,
note the compatibility information in the
[Installation & Upgrading instructions for Weave 1.1.0](https://github.com/weaveworks/weave/releases/tag/v1.1.0).

## Release v1.1.0

**Highlights**:

- `weave launch` now launches all weave components, simplifying
  startup.
- `weave status` has been completely revamped, with a much improved
  presentation of the information, and the option to select and output
  data in JSON.
- weaveDNS has been rewritten and embedded in the router. The new
  implementation simplifies configuration, improves performance, and
  provides fault resilience for services.
- the weave Docker API proxy now provides an even more seamless user
  experience, and enables easier integration of weave with other
  systems such as kubernetes.
- many usability improvements
- a few minor bug fixes, including a couple of security
  vulnerabilities

More details in the
[change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.1.0).

## Release 1.0.3

This release contains a weaveDNS feature enhancement as well as minor fixes for
improved stability and robustness.

More details in the
[change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.0.3).

The release is fully compatible with other 1.0.x versions, so existing
clusters can be upgraded incrementally.

## Release 1.0.2

This release fixes a number of bugs, including some security
vulnerabilities in the Weave Docker API proxy, hangs and failures in
address allocation, and sporadic failures in name resolution.

More details in the
[change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.0.2).

The release is fully compatible with other 1.0.x versions, so existing
clusters can be upgraded incrementally.

## Release 1.0.1

This is a bug fix release, addressing the following issue:

- executing `docker run` from a Docker 1.7 client against the weave
  proxy would report a `no such image` error when the requested image
  wasn't present on the Docker host, instead of downloading it.

## Release 1.0.0

**Highlights**:

- It is now easier than ever to start containers and for them to
  communicate, across multiple hosts. Automatic IP address allocation,
  and name resolution via weaveDNS are now enabled by default, and the
  proxy has become more fully-featured. In short, once weave has been
  launched the following is possible:

      ````
      host1$ docker run --name=pingme -dti ubuntu
      host2$ docker run -ti ubuntu
      root@d11e9287f65b:/# ping pingme
      ````
- Containers can now be
  [load-balanced](http://docs.weave.works/weave/latest_release/weavedns.html#load-balancing)
  easily.
- IP address allocation is now available across [multiple
  subnets](http://docs.weave.works/weave/latest_release/ipam.html#range),
  and hence can be employed when running multiple, isolated
  applications.
- The proxy now supports [TLS
  connections](http://docs.weave.works/weave/latest_release/proxy.html#tls),
  enabling its deployment when the communication between docker
  clients and the server must be secured.

There are many other new features, plus the usual assortment of bug
fixes and improvements under the hood. More detail below and in the
[change log](https://github.com/weaveworks/weave/issues?q=milestone%3A1.0.0).

*NB: This release changes the weave protocol version. Therefore, when
 upgrading an existing installation, all hosts need to be upgraded in
 order to for them to be able to communicate and form a network.*

## Release 0.11.2

This is a bug fix release, addressing the following issues:

- `weave run` did not respect DOCKER_CLIENT_ARGS. #855
- Negative result cache did not expire if requeried within TTL. #845

More details in the [change
log](https://github.com/weaveworks/weave/issues?q=milestone%3A0.11.2).

*NB: This release does not change the weave protocol version.
Therefore, when upgrading an existing 0.11 installation incrementally,
connectivity between peers will be retained.*

## Release 0.11.1

This is a bug fix release, addressing the following issues:

- The IP Allocator could crash in some relatively rare
  circumstances. #782/#783.
- When the proxy failed to attach a container to the weave network,
  there was no failure indication and descriptive error anywhere, and
  the application process would still start. Now an error is reported
  to the client (i.e. typically the Docker CLI), recorded in the proxy
  logs, and the container is terminated. #788/#799.
- `weave launch-proxy --with-ipam` failed to configure the entrypoint
  and DNS unless a (possibly blank) `WEAVE_CIDR` was
  specified. Affected containers could start the application process
  w/o the weave network interface being available, and without
  functioning name resolution for the weaveDNS domain.
  #744/#747/#751/#752
- The `weave status` output for the IP Allocator was misleadingly
  conveying a sense of brokenness when no IP allocation requests had
  been made yet. #787/#801
- When invoking `weave launch-proxy` twice, the second invocation
  would output a blank line and terminate with a success exit
  status. Now it reports that the proxy is already running and exits
  with a non-zero status. #767/#780
- `weave launch-proxy` was not respecting WEAVEPROXY_DOCKER_ARGS, so
  any user-supplied custom configuration for the weaveproxy container
  was ignored. #755/#780
- The proxy was not intercepting API calls to the unversioned (1.0)
  Docker Remote API. Hence none of the weave functionality was
  available when Docker clients were using that version of the
  API. #770/#774/#777,#809
- The proxy would crash when certain elements of the
  `/containers/create` JSON were missing. We are not aware of any
  Docker clients doing this, but it's been fixed regardless. #778/#777

More details in the [change
log](https://github.com/weaveworks/weave/issues?q=milestone%3A0.11.1).

*NB: This release does not change the weave protocol version.
Therefore, when upgrading an existing 0.11 installation incrementally,
connectivity between peers will be retained.*

## Release 0.11.0

**Highlights**:

- **automatic IP Address Management (IPAM)**, which allows application
  containers to be started and attached to the weave network without
  needing to supply an IP address.
- **proxy** for automatically attaching containers started with
  ordinary `docker run`, or the Docker remote API, to the weave
  network.
- ability to **add/remove extra DNS records**.
- performance and scalability improvements
- fixes for a small number of bugs discovered during testing

More detail in the [change log](https://github.com/weaveworks/weave/issues?q=milestone%3A0.11.0)

*NB: This release changes the weave protocol version. Therefore, when
upgrading an existing installation, all hosts need to be upgraded in
order to for them to be able to communicate and form a network.*

## Release 0.10.0

**Highlights**:

- **bug fixes**, in particular eradicating several deadlocks that could
  cause peer connection failures and hangs.
- **performance and scalability improvements** to the weave control plane,
  as a result of which it is now possible to construct much larger
  weave networks than previously.
- **improved installation and administration**, particularly the
  introduction of remote execution of the weave script in a container,
  permitting fully containerised deployment of weave.
- **improved diagnostics**, such as the reporting of connection failures
  in `weave status`.
- **new weaveDNS features**, most notably the caching of DNS records
  for application containers, which makes finding container IP
  addresses via weaveDNS much faster.

More detail in the [change log](https://github.com/weaveworks/weave/issues?q=milestone%3A0.10.0)

*NB: This release changes the Weave Docker image names. To upgrade
from an older version, 1) stop all application containers, 2) run
`weave reset` from the old version to remove all traces of weave, and
only then 3) install the new version.*

## Release 0.9.0

- Improve WeaveDNS to the point where it can act as the name server
  for containers in nearly all situations.

- Diagnose and report peer connectivity more comprehensively.

- Adapt to changes in topology - adding & removing of weave peers,
  disruption of connectivity - more rapidly.

- Cope with delays in downloading/running docker images/containers
  required for weave operation.

See the
[complete change log](https://github.com/weaveworks/weave/issues?q=milestone%3A0.9.0)
for more details.

## Release 0.8.0

- Align script and image version. When the `weave` script has a
  version number, i.e. it is part of an official release, it runs
  docker images matching that version. Thus the script and image
  versions are automatically aligned. Unversioned/unreleased `weave`
  scripts run the 'latest'-tagged image versions.

- Eliminate dependency on ethtool and conntrack. Instead of requiring
  these to be installed on the host, weave invokes them via a
  `weavetools` docker image that contains minimally packaged versions
  of these utilities.

- New `weave setup` command. This downloads all docker images used by
  weave. Invoking this is strictly optional; its main purpose is to
  facilitate automated installation of weave and preventing delays in
  subsequent weave command execution due to image downloading.

## Release 0.7.0

This is the first release assigned a version number.

When downloading weave you now have the following choices...

1. a specific version, e.g. https://github.com/weaveworks/weave/releases/download/v0.7.0/weave
2. latest released version: https://github.com/weaveworks/weave/releases/download/latest_release/weave
3. most recent 'master' commit: https://raw.githubusercontent.com/weaveworks/weave/master/weave

Previously the only documented download location was (3). We recommend
that any automated scripts using that be changed to either (1) or (2).
