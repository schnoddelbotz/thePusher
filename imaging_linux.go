package main

import (
	"fmt"
	"github.com/kardianos/osext"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

const (
	TC_VANILLA_ISO = "TinyCore-Vanilla.iso"
)

func createNBI() {
	log.Println("Creating netboot image using TinyCore 7.2/amd64 in ./pusher-nbi")

	selfPath, _ := osext.Executable()
	log.Printf("Integrating thePusher from %s", selfPath)
	log.Printf("Using thePusher master: %s", pusherIP)

	myUid := os.Geteuid()
	if myUid != 0 {
		log.Fatal("Error: This script must be executed as root (uid 0)")
	}

	// 0. prepare build env in ./pusher-nbi
	if _, err := os.Stat("./pusher-nbi"); err == nil {
		log.Fatal("./pusher-nbi directory already exists; please remove manually")
	}
	execFatal("mkdir", "pusher-nbi")
	os.Chdir("./pusher-nbi")

	log.Println("Downloading TinyCore")
	//downloadTinyCore("http://tinycorelinux.net/7.x/x86_64/release/CorePure64-7.2.iso")
	downloadTinyCore("http://distro.ibiblio.org/tinycorelinux/7.x/x86_64/release/CorePure64-7.2.iso")

	os.Chdir("./pusher-nbi")

	execFatal("mkdir", "-p", "mnt", "new")
	execFatal("mount", "-o", "loop", TC_VANILLA_ISO, "mnt")

	// 1. unpack vanilla iso initrd for modifications
	os.Chdir("new")
	execFatal("sh", "-c", "zcat ../mnt/boot/corepure64.gz | cpio -i -H newc -d")

	// install thePusher and make TC run it at init
	execFatal("cp", selfPath, "sbin/thePusher")
	writePusherSh(pusherIP, "sbin/startPusher")
	execFatal("chmod", "+x", "sbin/startPusher")
	writeInit(pusherIP)
	execFatal("chmod", "+x", "init")

	// make tc ld.so work with thePusher built on fedora
	execFatal("ln", "-s", "lib", "lib64")

	// 2. re-pack initrd with modifications
	// see: http://wiki.tinycorelinux.net/wiki:remastering
	execFatal("sh", "-c", "find | sudo cpio -o -H newc | gzip -9 > ../tinycore.gz")
	os.Chdir("..")
	execFatal("cp", "mnt/boot/vmlinuz64", ".")

	// 3. create bootable .iso for vm testing purposes
	execFatal("cp", "-R", "mnt", "newiso")
	execFatal("cp", "tinycore.gz", "newiso/boot/corepure64.gz")
	execFatal("perl", "-pi", "-e", "s@^timeout.*@timeout 30@", "newiso/boot/isolinux/isolinux.cfg")
	execFatal("mkisofs", "-l", "-J", "-R", "-V", "TC-custom", "-no-emul-boot",
		"-boot-load-size", "4", "-boot-info-table", "-b", "boot/isolinux/isolinux.bin",
		"-c", "boot/isolinux/boot.cat", "-o", "tinycore.iso", "newiso")

	// 4. clean up
	execFatal("umount", "mnt")
	execFatal("rm", "-rf", "mnt", "new", "newiso")
}

func downloadTinyCore(url string) {
	outputFile := TC_VANILLA_ISO
	out, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("Failed to create output file %s: %s", outputFile, err)
	}
	defer out.Close()
	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("Failed to download %s: %s", url, err)
	}
	defer resp.Body.Close()
	bytesWritten, err := io.Copy(out, resp.Body)
	if err != nil {
		log.Fatalf("Failed to save %s: %s", outputFile, err)
	}
	log.Printf("%s (%d bytes) successfully downloaded", outputFile, bytesWritten)
}

func writeInit(masterIP string) {
	initScriptTemplate := `#!/bin/sh
MASTER="PUSHER_MASTER_IP"
mount proc
grep -qw multivt /proc/cmdline && sed -i s/^#tty/tty/ /etc/inittab

if ! grep -qw superuser /proc/cmdline; then
  
  if grep -qw thePusher /proc/cmdline; then
	  MASTER=$(sed -E 's/.*thePusher=([^ ]+).*/\1/' /proc/cmdline)
	fi
	if [ -z "$MASTER" ]; then
	  if [ -z "$MASTER" ]; then
	  	echo "FATAL ERROR: thePusher master neither hard-coded nor in /proc/cmdline"
	  	sleep 30
	  fi
	fi

  if grep -qw putImage /proc/cmdline; then
  	IMG2PUT=$(sed -E 's/.*putImage=([^ ]+).*/\1/' /proc/cmdline)
  	sed -i "s@tty1.*@tty1::respawn:/sbin/startPusher put-image -p $MASTER -i $IMG2PUT@" /etc/inittab
  	echo "thePusher: /etc/inittab set up for: put-image"
  else
  	sed -i "s@tty1.*@tty1::respawn:/sbin/startPusher client -p $MASTER@" /etc/inittab
  	echo "thePusher: /etc/inittab set up for: put-image"
  fi

fi
## regular / default TinyCore code below
if ! grep -qw noembed /proc/cmdline; then

  inodes=` + "grep MemFree /proc/meminfo | awk '{print $2/3}' | cut -d. -f1" + `

  mount / -o remount,size=90%,nr_inodes=$inodes
  umount proc
  exec /sbin/init
fi
umount proc
if mount -t tmpfs -o size=90% tmpfs /mnt; then
  if tar -C / --exclude=mnt -cf - . | tar -C /mnt/ -xf - ; then
    mkdir /mnt/mnt
    exec /sbin/switch_root mnt /sbin/init
  fi
fi
exec /sbin/init`
	initScript := strings.Replace(initScriptTemplate, "PUSHER_MASTER_IP", masterIP, -1)
	f, err := os.Create("init")
	if err != nil {
		log.Fatalf("Cannot write to init: %s", err)
	}
	defer f.Close()
	fmt.Fprint(f, initScript)
}

func writePusherSh(masterIP string, destPath string) {
	f, err := os.Create(destPath)
	if err != nil {
		log.Fatalf("Cannot write to %s: %s", destPath, err)
	}
	defer f.Close()
	fmt.Fprint(f, `#!/bin/sh
    echo thePusher waiting for network...
    sleep 10
    ifconfig eth0
    /sbin/thePusher $@
  `)
	f.Sync()
}
