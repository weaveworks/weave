---
title: Managing Domains
layout: default
---

The following topics are discussed:

* [Configuring the domain search path](#domain-search-path)
* [Using a different local domain](#local-domain)

## <a name="domain-search-path"></a>Configuring the domain search paths

If you don't supply a domain search path (with `--dns-search=`),
`weave run ...` tells a container to look for "bare" hostnames, like
`pingme`, in its own domain (or in `weave.local` if it has no domain).
That's why you can just invoke `ping pingme` above -- since the
hostname is `ubuntu.weave.local`, it will look for
`pingme.weave.local`.

If you want to supply other entries for the domain search path,
e.g. if you want containers in different sub-domains to resolve
hostnames across all sub-domains plus some external domains, you need
*also* to supply the `weave.local` domain to retain the above
behaviour.

```bash
docker run -ti \
  --dns-search=zone1.weave.local --dns-search=zone2.weave.local \
  --dns-search=corp1.com --dns-search=corp2.com \
  --dns-search=weave.local ubuntu
```

## <a name="local-domain"></a>Using a different local domain

By default, weaveDNS uses `weave.local.` as the domain for names on the
Weave network. In general users do not need to change this domain, but
you can force weaveDNS to use a different domain by launching it with
the `--dns-domain` argument. For example,

```bash
$ weave launch --dns-domain="mycompany.local."
```

The local domain should end with `local.`, since these names are
link-local as per [RFC6762](https://tools.ietf.org/html/rfc6762),
(though this is not strictly necessary).


 * [How Weave Finds Containers](/site/weave-docker-api/how-works-weavedns.md)
 * [Load Balancing and Fault Resilience with weavedns](/site/weave-docker-api/load-balance-fault-weavedns.md)
 * [Managing Domain Entries](/site/weavedns/managing-entries-weavedns.md)
