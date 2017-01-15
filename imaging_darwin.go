package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
)

func createNBI() {
	pusherBaseSystemDMG := "/tmp/pusher_base/BaseSystem.sparseimage"
	efiBootPath := "/Volumes/Recovery HD/com.apple.recovery.boot/boot.efi"
	prelinkedKernelPath := "/Volumes/Recovery HD/com.apple.recovery.boot/prelinkedkernel"
	nbiOutputFolder := "/tmp/thePusher.nbi"
	recoveryPartition, err := getRecoveryPartition()
	if err != nil {
		log.Fatal("Error finding recovery partition: %s", err)
	}
	log.Printf("Found recovery partition: %s", recoveryPartition)
	execFatal("diskutil", "mount", "readOnly", recoveryPartition)
	execFatal("mkdir", "/tmp/pusher_base")
	execFatal("hdiutil", "convert", "/Volumes/Recovery HD/com.apple.recovery.boot/BaseSystem.dmg",
		"-format", "UDSP", "-o", pusherBaseSystemDMG)
	execFatal("hdiutil", "resize", "-size", "8G", pusherBaseSystemDMG)
	execFatal("ditto", "--rsrc", efiBootPath, "/tmp/pusher_base/booter")
	execFatal("ditto", "--rsrc", prelinkedKernelPath, "/tmp/pusher_base/kernelcache")
	execFatal("diskutil", "unmount", recoveryPartition)

	execFatal("mkdir", nbiOutputFolder)
	execFatal("mv", pusherBaseSystemDMG, nbiOutputFolder+"/NetInstall.sparseimage")
	execFatal("ln", "-s", "NetInstall.sparseimage", nbiOutputFolder+"/NetInstall.dmg")

	// mount NBI's sparse image
	mountedNbiDevice, err := mountSparseImageAndGetPartition(nbiOutputFolder + "/NetInstall.sparseimage")
	if err != nil {
		log.Fatalf("Failed to get slice for mounted %s", nbiOutputFolder+"/NetInstall.sparseimage")
	}
	execFatal("diskutil", "renameVolume", mountedNbiDevice, "thePusher")
	// NBI will now live under /Volumes/thePusher

	// add kernel(+cache) to .nbi
	execFatal("mkdir", "-p", nbiOutputFolder+"/i386/x86_64")
	execFatal("cp", "/Volumes/thePusher/System/Library/CoreServices/PlatformSupport.plist", nbiOutputFolder+"/i386")
	execFatal("cp", "/tmp/pusher_base/booter", "/Volumes/thePusher/usr/standalone/i386/boot.efi")
	execFatal("cp", "/tmp/pusher_base/booter", nbiOutputFolder+"/i386/booter")
	execFatal("cp", "/tmp/pusher_base/kernelcache", "/Volumes/thePusher/System/Library/PrelinkedKernels/prelinkedkernel")
	execFatal("cp", "/tmp/pusher_base/kernelcache", nbiOutputFolder+"/i386/x86_64/kernelcache")
	execFatal("chmod", "644", nbiOutputFolder+"/i386/x86_64/kernelcache")
	// modify .nbi sparse image to let it start thePusher...
	// tbd
	execFatal("hdiutil", "eject", "/Volumes/thePusher")

	// write nbiImageInfo
	writeNBImageInfo(nbiOutputFolder + "/NBImageInfo.plist")
	nbiInfoPlist := nbiOutputFolder + "/NBImageInfo"
	execFatal("defaults", "write", nbiInfoPlist, "Architectures", "-array", "i386") // ${ARCH}
	execFatal("defaults", "write", nbiInfoPlist, "DisabledSystemIdentifiers", "-array")
	execFatal("defaults", "write", nbiInfoPlist, "EnabledSystemIdentifiers", "-array")
	execFatal("defaults", "write", nbiInfoPlist, "Index", "-int", "1234")
	execFatal("defaults", "write", nbiInfoPlist, "Name", "thePusher")
	execFatal("defaults", "write", nbiInfoPlist, "Description", "thePusher")
	execFatal("defaults", "write", nbiInfoPlist, "Language", "English")
	execFatal("defaults", "write", nbiInfoPlist, "LanguageCode", "en")
	execFatal("defaults", "write", nbiInfoPlist, "osVersion", "10.12")
	execFatal("plutil", "-convert", "xml1", nbiInfoPlist+".plist")

}

func getRecoveryPartition() (string, error) {
	out, err := exec.Command("diskutil", "list").Output()
	if err != nil {
		log.Fatal(err)
	}
	sout := string(out)
	regex, err := regexp.Compile("Apple_Boot\\s+Recovery HD\\s+\\d+\\.\\d+\\s+MB\\s+(\\S+)")
	if err != nil {
		return "", errors.New("Couldn't compile regex")
	}
	result_slice := regex.FindStringSubmatch(sout)
	if len(result_slice) == 2 {
		return "/dev/" + result_slice[1], nil
	} else {
		return "", errors.New("regex didnt match on diskutil output")
	}
}

func mountSparseImageAndGetPartition(sparseImagePath string) (string, error) {
	// hdiutil attach thePusher.nbi/NetInstall.sparseimage
	out, err := exec.Command("hdiutil", "attach", sparseImagePath).Output()
	if err != nil {
		log.Fatal(err)
	}
	sout := string(out)
	regex, err := regexp.Compile("/dev/disk(\\d+)s1")
	if err != nil {
		return "", errors.New("Couldn't compile regex")
	}
	result_slice := regex.FindStringSubmatch(sout)
	if len(result_slice) == 2 {
		return "/dev/disk" + result_slice[1] + "s1", nil
	} else {
		return "", errors.New("regex didnt match on hdiutil output")
	}
}

func writeNBImageInfo(destPath string) {
	log.Printf("Writing %s", destPath)
	ImageInfo := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
        <key>BootFile</key>
        <string>booter</string>
        <key>BackwardCompatible</key>
        <false/>
        <key>IsDefault</key>
        <false/>
        <key>IsEnabled</key>
        <true/>
        <key>IsInstall</key>
        <true/>
        <key>Kind</key>
        <integer>2</integer>
        <key>RootPath</key>
        <string>NetInstall.dmg</string>
        <key>SupportsDiskless</key>
        <false/>
        <key>Type</key>
        <string>NFS</string>
</dict>
</plist>`
	f, err := os.Create(destPath)
	if err != nil {
		log.Fatalf("Cannot write to %s: %s", destPath, err)
	}
	defer f.Close()
	fmt.Fprint(f, ImageInfo)
}
