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
