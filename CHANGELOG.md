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
