package main

import (
	"github.com/hashicorp/hcl"
	"io/ioutil"
	"log"
	"os"
)

type Config struct {
	// hosts_allow []string IPs
	Images       []Image       `hcl:"image"`
	Clientgroups []Clientgroup `hcl:"clientgroup"`
}

type Image struct {
	Name        string `hcl:",key"`
	Filename    string `hcl:"filename"`
	Comment     string `hcl:"comment"`
	Md5         string `hcl:"md5"`
	Destination string `hcl:"destination"`
	Type        string `hcl:"type"`
	Compression string `hcl:"compression"`
	PreImage    string `hcl:"preImage"`  // pass to bash -c prior to image restore [notyet]
	PostImage   string `hcl:"postImage"` // pass to bash -c post image restore [notyet]
	// size
}

type Clientgroup struct {
	Name  string   `hcl:",key"`
	Image string   `hcl:"image"`
	Hosts []string `hcl:"hosts"`
}

func readConfig(filename string) {
	var result Config
	fileContents, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatal("Cannot read config file ", filename)
	}
	hclParseTree, err := hcl.ParseBytes(fileContents)
	if err != nil {
		log.Fatal("Config parser error: ", err)
	}
	if err := hcl.DecodeObject(&result, hclParseTree); err != nil {
		log.Fatal("Error decoding config: ", err)
	}
	masterConfig = result
	verifyConfig()
}

func verifyConfig() {
	for _, grp := range masterConfig.Clientgroups {
		img := getImageByKey(grp.Image)
		if img.Name == "" {
			log.Fatalf("Invalid image %s for client group %s", grp.Image, grp.Name)
		}
		if img.Compression != COMP_NONE && img.Compression != COMP_BZIP2 {
			log.Fatalf("Invalid compression %s for image %s. Supported: '%s' and '%s'", img.Compression, grp.Image, COMP_NONE, COMP_BZIP2)
		}
		if _, err := os.Stat(imageStorage + "/" + img.Filename); os.IsNotExist(err) {
			log.Fatalf("Image '%s' of group %s does not exist", img.Filename, grp.Name)
		}
		if len(grp.Hosts) == 0 {
			log.Fatalf("Group %s has zero hosts defined", grp.Name)
		}
	}
}

func getImageByKey(key string) Image {
	// isn't there an easier way to look it up...?
	for _, img := range masterConfig.Images {
		if img.Name == key {
			return img
		}
	}
	var nullImage Image
	return nullImage
}

func getClientgroupByKey(key string) Clientgroup {
	// isn't there an easier way to look it up...?
	for _, grp := range masterConfig.Clientgroups {
		if grp.Name == key {
			return grp
		}
	}
	var nullGroup Clientgroup
	return nullGroup
}
