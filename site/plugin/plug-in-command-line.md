---
title: Plugin Command-line Arguments
menu_order: 10
---



If you need to give additional arguments to the plugin independently, don't
use `weave launch`, but instead run:

    $ weave launch-router [other peers]
    $ weave launch-plugin [plugin arguments]

The plugin command-line arguments are:

 * `--log-level=debug|info|warning|error` --tells the plugin
   how much information to emit for debugging.
 * `--no-restart` -- remove the default policy of `--restart=always`, if
   you want to control start-up of the plugin yourself
 * `--no-multicast-route` -- stops weave from adding a static IP route for
   multicast traffic onto its interface

By default, multicast traffic is routed over the weave network.
To turn this off, e.g. you want to configure your own multicast
route, add the `--no-multicast-route` flag to `weave launch-plugin`.


>Note: When using the Docker Plugin, there is no need to run eval $(weave env) to enable the Proxy. Because Weave is running as a plugin within Docker, the Weave Docker API Proxy, at present, cannot detect between networks.  

**See Also**

 * [Using the Weave Net Docker Network Plugin](/site/plugin.md)
 * [How the Weave Network Plugin Works](/site/plugin/plugin-how-it-works.md)
