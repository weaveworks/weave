# AWSVPC

## Today

- Change container type
- Fix breakage with DOCKER_BRIDGE_IP

- Extend the 900 tests that it would create container on the same host
- Test stop/launch <- does it trigger update? does it have to trigger update?
- Check destination in the route assertions
- Rebase master
- Rebase master

* Cleanup the code
* Get rid of monitor status
* Add the docs
* Change alloc.universe type to address.CIDR

- Add tests for proxy
* * Add tests for plugin

* Disable multiple subnets
* Check that the attach belongs to the main subnet

* Get rid of ethtool tx off for AWSVPC

- Fix `weave status`
---> * Use NewNull()
---> * Bridge Interface String()
* Fix `weave report` re "no bridge networking"

* Read on VPC routing tables again.
* When starting AWS VPC Monitor, fail early if there are entries from the subnet // not possible because
  some other peer might pushed entries. I could check for local entries though,
  but it's problematic because of `weave stop` && `weave launch`.

* Check how callico does the networking
* Check how flannel does the networking
* Check how swarm does networking on AWSVPC

- There is a bug when `weave reset` does not clean the rt properly <= probably
  because of weave stop exec upon end_suite
- weave stop -> weave reset does not cleanup the tables, does it notify other
  IPAM peers?

- Check why the first packet takes ages to send && maybe check with gw setup
* Check why there are not so many packets on ARP
* Check whether flooding increases with proxy_delay--
* What happens when we have thousands containers on the same host, does
  flooding bites us?

* Benchmark to see whether proxy_arp sux or not.
* What happens when we run fastdp and awsvpc at the same time.
* Enable --awsvpc in other tests
* Test re-attach of containers
* Bring $HOST3

## aws.sh

* do not delete keys
* create a separate VPC for each run
* AWS=1 might be set for GCE machines, add the reset cmd

## Misc

Invalid log msg:
DEBU: 2016/05/27 10:01:15.116349 [allocator 62:5e:2b:83:97:6e]: Allocated
10.32.0.1 for weave:expose in 10.32.0.0/12
DEBU: 2016/05/27 10:01:15.528758 [http] GET /status
DEBU: 2016/05/27 10:01:15.547612 [http] PUT
/ip/weave:expose/10.32.0.1/12?noErrorOnUnknown=true
DEBU: 2016/05/27 10:01:15.547754 [allocator 62:5e:2b:83:97:6e]: Re-Claimed
10.32.0.1/12 for weave:expose

weave report:
"Interface": "\u003cno bridge networking\u003e",
