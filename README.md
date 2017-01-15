# thePusher

makes your linux and mac disk images flow to the masses

## Introduction

thePusher is intended to deploy file-system images to client machines.
Images are dd images, optionally compressed using bzip2.
To avoid a server network bottleneck, images are
streamed from the server to the first client, who writes the image
to disk *while* streaming the incoming image to the next client.

```
+----------+    +----------+    +----------+    +----------+
|          |    |          |    |          |    |          |
|  master  |    | client 1 |    | client 2 |    | client n |
|          |    |          |    |          |    |          |
|  pushes  |    | writes & |    | writes & |    |  writes  |
|          |    |  pushes  |    |  pushes  |    |          |
+--------+-+    +-^------+-+    +-^------+-+    +-^--------+
         |        |      |        |      |        |
         +--------+      +--------+      +--------+
```

To allow imaging, thePusher supports creation of netboot images,
based on TinyCore Linux (or the recovery partition for macs).
It is assumed that PCs boot using PXE. Clients register
their ready-state with the master upon successful PXE boot;
if all clients to be imaged have booted up and connected to
their "neighbor" successfully, the master starts streaming the image.
Given a solid network switch is used, time for image restore should
not take longer with increasing client count.

## Basic usage

- Set up DHCP / pxelinux (or ipxe) for your clients
- Use a client and `thePusher create-nbi` to create a netboot image
- Set up thePusher master configuration file by adjusting the
  [example](thePusher-config_example.hcl)
- Run thePusher master on a machine that will act as server
- Install and set up desired OS to be deployed on a client
- Boot the client to be imaged into the netboot image
  using putImage=... option to create a disk image and put it on
  master
- Open http://&lt;master&gt;:8080 to review configuration and monitor progress
- Let clients boot using PXE to start image restoration

The command `thePusher create-nbi` creates the NBI in a fully
automated fashion (i.e. downloads TinyCore, installs thePusher
into the image, sets your master's IP inside the image and
re-creates the TinyCore initrd). You may alternatively create a 'generic'
NBI without a 'built-in' master setting and instead specify the
master's IP through pxelinux kernel command-line.

## Installation

Download a [release](../../releases) binary for your OS or just:

```bash
go get github.com/schnoddelbotz/thePusher
```

You may also grab a pre-built TinyCore netboot image from the [release](../../releases)
page. This is optional -- you can use `thePusher create-nbi` to brew your own;
it just requires an existing Linux box with `mkisofs` installed on it and has
the advantage of hard-coding your master's IP into the boot image. When choosing
the pre-built image, you'll always have to specify the master's IP on the
kernel command line. The kernel command-line value will always override the
hard-coded master selection.

### Short Linux image creation and restoration walk-through

To get started, choose whether you want to create the image using a VM (very easy)
or using PXE boot (requires DHCP server setup, prepared for netbooting). The next
steps explain the VM approach for brevity -- but required PXE setup is also shown.

#### Set up a master

1. Install thePusher as outlined above, i.e. `thePusher` should be available in your $PATH
2. Create a directory to serve master's images (e.g. `/scratch/images`)
3. Optionally, create a directory to serve scripts to be executed pre/post-imaging
   (e.g. /scratch/scripts)
4. Copy the [example configuration](thePusher-config_example.hcl) to image directory
   (e.g. `/scratch/images` as created above), name it `thePusher-config.hcl`.
5. Run the master: `thePusher -verbose master -S /scratch/images -s /scratch/scripts`

#### Create Client to be imaged

1. Create a VM and install your favorite Linux distro on the VM's disk
2. Shut down the VM and re-configure it to reboot from thePusher's `tinycore.iso`
3. At the isolinux boot command, type `corepure64 putImage=/dev/sda thePusher=1.2.3.4`
   to create an image of /dev/sda and store it on master with IP 1.2.3.4.
4. Wait until image of /dev/sda has been put on master -- done!
5. On the master, you should find a file "sda" inside the image directory.
   You may decide to compress it using bzip2
6. Create an entry for the new image in `thePusher-config.hcl`
7. Create a clientgroup that references the image and lists your desired clients
8. Restart thePusher for the configuration changes to take effect

#### Restore image

1. Point a web browser to http://YOUR-MASTER:8080/ to monitor progress; select
   client group created before.
2. Let your VM boot `tinycore.iso`
3. If using a pre-built `tinycore.iso`, specify master IP on isolinux boot command
   line, e.g. `corepure64 thePusher=1.2.3.4`. Using a custom built netboot image,
   you can let the machine just boot -- it will directly boot into restore mode
4. Wait until restore has completed. Be happy.

When using PXE instead of `tinycore.iso`, you just have to use the same options
as used above for the kernel command line. An example pxelinux.cfg might look like this:

```
LABEL thePusher-Restore
LINUX vmlinuz64
APPEND initrd=tinycore.gz thePusher=1.2.3.4

LABEL thePusher-PUT-Image
LINUX vmlinuz64
APPEND initrd=tinycore.gz thePusher=1.2.3.4 putImage=/dev/sda
```

## Status

thePusher is a pure fun project to get me into Go. It *works for me* as
intended on VMs, but I haven't tested it yet on a bigger group of
real clients. Use at your own risk!

### Open issues / To-Do

- Web interface could/should support configuration; it's currently only
  used for monitoring client status. It might be nice to show progress, too.
- For a reason yet unknown, bzip2 compressed images perform worse than uncompressed ones.
- On macOS, the `createNBI` action creates a somewhat usable netboot image,
  but it does not start thePusher yet. It seems to be difficult to
  [boot into console mode](http://apple.stackexchange.com/questions/119027/login-directly-to-terminal-instead-of-gui) on Sierra.

## Alternatives

[CloneZilla](http://clonezilla.org/) uses multicasting to achieve
performance similar to thePusher. It may be more mature in several
areas, but maybe also more complex to set up. Whether it performs better
than thePusher mainly depends on your switch hardware and
cluster size. ThePusher *might* perform better with larger clusters -- YMMV.

## LICENSE

[thePusher](https://www.youtube.com/watch?v=3XqyGoE2Q4Y) is
under MIT license. (c) schnoddelbotz 2016.
