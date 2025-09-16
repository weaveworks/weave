---
title: Launching Containers With Weave Run (without the Proxy)
menu_order: 60
search_type: Documentation
---

In prior versions of Weave Net, we supported a command `weave run`
which would run a container and attach it to the Weave Network.

This command always had limitations, and now we have Docker plugins
and the Weave Net Docker API Proxy which do not have those
limitations, we have removed `weave run`.

If you have scripts using `weave run`, you can achieve exactly the
same effect by combining `docker run` and `weave attach`; see
[Dynamically Attaching and Detaching
Containers]({{ '/tasks/manage/dynamically-attach-containers' | relative_url }}) for
details.

The `weave start` and `weave restart` commands have also been removed,
for the same reasons.
