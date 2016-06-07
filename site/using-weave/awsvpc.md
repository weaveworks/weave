---
title: Using IP Routing on an Amazon Web Services Virtual Private Cloud
menu_order: 110
---

If you are running your container infrastructure entirely within
Amazon Web Services (AWS) Elastic Compute Cloud (EC2), then you can
choose AWS-VPC mode to connect your containers without any overlay, so
they can operate very close to the full speed of the underlying
network.

In this mode, Weave Net still manages IP addresses and connects
containers to the network, but instead of wrapping up each packet and
sending it to its destination, Weave Net instructs the AWS network
router which ranges of container IP addresses live on which instance.

###Configuring your EC2 instances to use Weave AWS-VPC mode

First, your AWS instances need to be given access to change the route
table.  Give them an IAM Role which has this access; if you have an
existing role then extend it or create a new one with a policy
allowing the necessary actions:

```
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ec2:CreateRoute",
                "ec2:DeleteRoute",
                "ec2:ReplaceRoute",
                "ec2:DescribeRouteTables",
                "ec2:DescribeInstances"
            ],
            "Resource": [
                "*"
            ]
        }
    ]
}
```

Your Security Group must allow network traffic between instances: you
must open TCP port 6783 which Weave Net uses to manage the network,
and you must allow any ports which your own containers use. Remember:
there is no network overlay in this mode so IP packets with container
addresses will flow over the AWS network unmodified.

Also, you must disable the "Source/Destination check" on each machine,
because Weave will be operating with IP addresses outside of the range
allocated by Amazon.

###Using AWS-VPC mode

Launch Weave Net, adding the `--awsvpc` flag:

    $ weave launch --awsvpc [other hosts]

You still need to supply the names or IP addresses of other hosts in
your cluster.

###Limitations

- AWS-VPC mode does not inter-operate with other Weave Net modes; it
  is all or nothing.  In this mode, all hosts in a cluster must be AWS
  instances. We hope to ease this limitation in future.
- The AWS network does not support multicast.
- The number of hosts in a cluster is limited by the maximum size of
  your AWS route table.  This is limited to 50 entries though you
  can request an increase to 100 by contacting Amazon.
- All your containers must be on the same network, with no subnet
  isolation. We hope to ease this limitation in future.

###Packet size (MTU)

The Maximum Transmission Unit, or MTU, is the technical term for the
limit on how big a single packet can be on the network. Weave Net
defaults to 1410 bytes which works across almost all networks, but you
can set a larger size for better performance.

The AWS network supports packets of 9000 bytes, so in AWS-VPC mode you
can run:

    $ WEAVE_MTU=9000 weave launch --awsvpc host2 host3

**See Also**

 * [Using Weave Net](/site/using-weave.md)
 * [Performance measurements](/blog/weave-docker-networking-performance-aws-vpc/)

(that last blog post doesn't exist yet)
