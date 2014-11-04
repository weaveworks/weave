# Weave DNS (service discovery) design

The model is that each host has a service that is notified of
hostnames and weave addresses for containers on the host. It binds to
the host bridge to answer DNS queries from local containers; anything
it can't answer, it asks with mDNS on the weave network.

The service is divided into two components: the first is the DNS
server, which serves the records it is told about, and otherwise asks
via mDNS on the weave network. The other component is the DNS updater,
specific to Docker, which monitors the running containers so to add
and remove records from the DNS server.

## DNS server API

The DNS server accepts HTTP requests on the following URL (patterns)
and methods:

`PUT /name/<identifier>/<ip-address>`

Put a record for an IP, bound to a host-scoped identifier (e.g., a
container ID), in the DNS database.

`DELETE /name/<identifier>/<ip-address>`

Remove a specific record for an IP and host-scoped identifier.

`DELETE /name/<identifier>`

Remove all records for the host-scoped identifier.

`GET /name/`

List all the records in the database.

`GET /name/<identifier>`

List all the records for a host-scoped identifier.

`GET /status`

Give the server status.

### Record structure

The record structure is *either* form fields (useful for scripting
PUTs), or a JSON object with the same fields. The fields are:

```js
    {
        "fqdn": string,
        "routing_prefix": number,
        "local_ip": string
    }
```

> Also TTL?

## DNS server behaviour

The DNS server listens for the HTTP requests as above, and maintains a
database of host-local names accordingly.

It also listens on the host-local network for DNS queries (port
53). Anything that is in its database it responds to immediately. For
other requests, it asks using multicast DNS on the weave network.

Meanwhile, the DNS server is listening for mDNS queries, and responds
if it the queried name is in its database.

### Caching mDNS answers

The DNS server will hear the answers to its own and other servers'
mDNS queries sent on the weave network. It can cache these, to respond
to local DNS requests; the cache is consulted before using mDNS.

## DNS updater

The updater component uses the Docker remote API to monitor containers
coming and going, and tells the DNS server to update its records via
its HTTP interface. It does not need to be attached to the weave
network.

The updater starts by subscribing to the events, and getting a list of
the current containers. Any containers given a domain ending with
".weave" are considered for inclusion in the name database.

When it sees a container start or stop, the updater checks the weave
network attachment of the container, and updates the DNS server.

> How does it check the network attachment from within a container?

> Will it need to delay slightly so that `attach` has a chance to run?
> Perhaps it could put containers on a watch list when it's noticed
> them.

## Container startup

> TBD

## Grouping and load balancing

> TBD

## Extensions

 * An updater that checks health more specifically, e.g., by testing
   that a particular port is being listened to, or that it can get a
   200 OK. These could be encoded in the container environment
   variables, as per registrator.
