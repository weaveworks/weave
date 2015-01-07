# Weave Rendezvous service

The Weave rendevous service is responsible for finding new peers as well
as announcing the local weave router in a group.

## Using the Rendezvous service

You can use the `join` command for specifying a group to join.
For example:

```bash
$ weave launch
$ weave join mdns://somedomain
...
```

