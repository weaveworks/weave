---
title: Using Weave with Systemd
layout: default
---


Having installed `weave` as per [Installing Weave](/site/installing-weave.md), you might find it convenient to configure the
init daemon to start Weave on boot. Most recent Linux distribution releases are
shipping with [systemd](http://www.freedesktop.org/wiki/Software/systemd/). The information below should provide you with some
initial guidance on getting a Weave service configured on a systemd-based OS.

## Weave Service Unit and Configuration

A regular service unit definition for Weave is shown below. This file is
normally placed in `/etc/systemd/system/weave.service`.

    [Unit]
    Description=Weave Network
    Documentation=http://docs.weave.works/weave/latest_release/
    Requires=docker.service
    After=docker.service
    [Service]
    EnvironmentFile=-/etc/sysconfig/weave
    ExecStartPre=/usr/local/bin/weave launch $PEERS
    ExecStart=/usr/bin/docker attach weave
    ExecStop=/usr/local/bin/weave stop
    [Install]
    WantedBy=multi-user.target


To specify the addresses or names of other Weave hosts to join the network, 
create the `/etc/sysconfig/weave` environment file using the following format:

    PEERS="HOST1 HOST2 .. HOSTn"

You can also use the [`weave connect`](/site/using-weave/finding-adding-hosts-dynamically.md) command to add participating hosts dynamically.

Additionally, if you want to enable [encryption](/site/using-weave/security-untrusted-networks.md) you can specify a
password with e.g. `WEAVE_PASSWORD="wfvAwt7sj"` in the `/etc/sysconfig/weave` environment file, and it will get picked up by
Weave on launch. Recommendations for choosing a suitably strong password can be found [here](/site/using-weave/security-untrusted-networks.md).

You can now launch Weave using

    sudo systemctl start weave

To ensure Weave launches after reboot, run:

    sudo systemctl enable weave

For more information on systemd, please refer to the documentation supplied
by your distribution of Linux.

## SELinux Tweaks

If your OS has SELinux enabled and you want to run Weave as a systemd unit,
then follow the instructions below. These instructions apply to
CentOS and RHEL as of 7.0. On Fedora 21, there is no need to do this.

Once `weave` is installed in `/usr/local/bin`, set its execution
context with the commands shown below. You will need to have the
`policycoreutils-python` package installed.

    sudo semanage fcontext -a -t unconfined_exec_t -f f /usr/local/bin/weave
    sudo restorecon /usr/local/bin/weave

**See Also**

 * [Using Weave Net](/site/using-weave/intro-example.md)
 * [Getting Started Guides](http://www.weave.works/guides/)
 * [Features](/site/features.md)
 * [Troubleshooting](/site/troubleshooting.md)
