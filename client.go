package main

import (
	"bufio"
	"compress/bzip2"
	"encoding/json"
	"fmt"
	"github.com/fatih/color"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var red = color.New(color.FgRed).SprintFunc()
var blue = color.New(color.FgBlue).SprintFunc()
var cTask ClientTask

func runClient() {
	if pusherIP == "" {
		fmt.Printf("%s: -pusher flag required. Use -h for help.\n", red("ERROR"))
		return
	}

	fmt.Printf("\nthePusher client version %s starting...\n\n", blue(thePusherVersion))

	// get client task
	cTask = getTask()

	// execute pre-imaging script if defined
	if cTask.ImageInfo.PreImage != "" {
		log.Printf("Running preImage command: %s", cTask.ImageInfo.PreImage)
		execScript(cTask.ImageInfo.PreImage)
		log.Printf("preImage script completed")
	}

	// report_ready() -- defer by 2 seconds; start image reception first
	go func() {
		time.Sleep(2 * time.Second)
		if cTask.ClientInfo.Neighbor == "" {
			ssurl := fmt.Sprintf("http://%s:8080/startStream/%s", pusherIP, cTask.ClientInfo.Group)
			log.Printf("Requesting startStream from master: %s", ssurl)
			ssreq, err := http.Get(ssurl)
			if err != nil || ssreq.StatusCode != 200 {
				log.Fatalf("FAILED to request startStream from %s (%d)", ssurl, ssreq.StatusCode)
			}
			ssreq.Body.Close()
		}
	}()

	// run http listener for image reception
	log.Printf("Waiting for PUT request ...")
	http.HandleFunc("/receiveImage", receiveImageHandler)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func receiveImageHandler(w http.ResponseWriter, request *http.Request) {
	log.Printf("/receiveImage starting (source: %s)", request.RemoteAddr)
	outfile, err := os.Create(cTask.ImageInfo.Destination)
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}
	defer outfile.Close()

	if cTask.ClientInfo.Neighbor == "" {
		// WRITE LOCALLY ONLY
		log.Print("/receiveImage ... starting in local-write-only mode")
		go reportClientStatus(STATUS_BUSY)
		outfileWriter := bufio.NewWriter(outfile)
		if cTask.ImageInfo.Compression == COMP_BZIP2 {
			reader := bzip2.NewReader(request.Body)
			outfileWriter.ReadFrom(reader)
		} else {
			reader := bufio.NewReader(request.Body)
			reader.WriteTo(outfileWriter)
		}
		outfileWriter.Flush()
		outfile.Sync()
	} else {
		// WRITE LOCALLY AND STREAM TO NEIGHBOR
		// http://stackoverflow.com/questions/1821811/how-to-read-write-from-to-file
		url := fmt.Sprintf("http://%s:8080/receiveImage", cTask.ClientInfo.Neighbor)
		pr, pw := io.Pipe()
		fwdRequest, err := http.NewRequest("PUT", url, pr)
		fwdRequest.ContentLength = request.ContentLength
		if err != nil {
			log.Fatalf("CANNOT FORWARD: %s", err)
		}
		fwdClient := &http.Client{}

		// create forward connection
		go func() {
			log.Print("/receiveImage ... connection to neighbor set up")
			_, ferr := fwdClient.Do(fwdRequest)
			if ferr != nil {
				log.Fatalf("Forwarding error: %s", ferr)
			}
			pr.Close()
		}()
		log.Print("/receiveImage ... storing and forwarding now...")
		go reportClientStatus(STATUS_BUSY)

		// tee is a reader on request.body, copying to pipe-forward to neighbor
		tee := io.TeeReader(request.Body, pw)
		buf := make([]byte, 1048576)

		// Switch BZIP2 or not
		if cTask.ImageInfo.Compression == COMP_BZIP2 {
			// BZIP2
			log.Printf("/receiveImage ... starting in BZ2 forwarding mode (to: %s)", cTask.ClientInfo.Neighbor)
			decompReader := bzip2.NewReader(tee)
			// now store and forward...
			for {
				// read a chunk -- read on T (here, via decompReader) automatically writes to pipe
				n, err := decompReader.Read(buf)
				if err != nil && err != io.EOF {
					panic(err)
				}
				if n == 0 {
					break
				}
				// write a chunk to disk
				if _, err := outfile.Write(buf[:n]); err != nil {
					panic(err)
				}
			}
		} else {
			// NON-BZIP2
			log.Printf("/receiveImage ... starting in NONBZ2 forwarding mode (to: %s)", cTask.ClientInfo.Neighbor)
			// now store and forward...
			for {
				// read a chunk -- read on T automatically writes to pipe
				n, err := tee.Read(buf)
				if err != nil && err != io.EOF {
					panic(err)
				}
				if n == 0 {
					break
				}
				// write a chunk to disk
				if _, err := outfile.Write(buf[:n]); err != nil {
					panic(err)
				}
			}
		}

		// bzip2 or not...:
		log.Print("/receiveImage ... closing filehandles")
		outfile.Sync()
		pw.Close()
	}

	log.Print("/receiveImage completed")
	if cTask.ImageInfo.PostImage != "" {
		log.Printf("Running postImage command: %s", cTask.ImageInfo.PostImage)
		execScript(cTask.ImageInfo.PostImage)
		log.Printf("postImage script completed")
	}
	go reportClientStatus(STATUS_DONE_OK)
}

func getTask() ClientTask {
	log.Printf("Retrieving task from %s ...", pusherIP)
	apiUrl := fmt.Sprintf("http://%s:8080/getClientTask", pusherIP)
	response, err := http.Get(apiUrl)
	if err != nil {
		log.Fatalf("%s: Cannot contact master: %s", red("ERROR"), err)
	}
	if response.StatusCode == http.StatusNotFound {
		log.Fatalf("%s: Client not found in any groups on master (404)", red("ERROR"))
	} else if response.StatusCode != http.StatusOK {
		log.Fatalf("%s: Bad response from server (%d)", red("ERROR"), response.StatusCode)
	}
	defer response.Body.Close()
	var task ClientTask
	json.NewDecoder(response.Body).Decode(&task)
	task.ImageInfo.PreImage = strings.Replace(task.ImageInfo.PreImage, "#MASTER#", pusherIP, -1)
	task.ImageInfo.PostImage = strings.Replace(task.ImageInfo.PostImage, "#MASTER#", pusherIP, -1)
	printTask(task)
	return task
}

func execScript(script string) {
	cmd := exec.Command("sh", "-c", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func printTask(t ClientTask) {
	//info := color.New(color.FgBlack, color.BgHiWhite).SprintFunc()
	fmt.Printf("Image comment     : %s\n", t.ImageInfo.Comment)
	fmt.Printf("Image filename    : %s\n", t.ImageInfo.Filename)
	fmt.Printf("Image type        : %s\n", t.ImageInfo.Type)
	fmt.Printf("Image compression : %s\n", t.ImageInfo.Compression)
	fmt.Printf("Image md5         : %s\n", t.ImageInfo.Md5)
	fmt.Printf("Image destination : %s\n", t.ImageInfo.Destination)
	fmt.Printf("PreImage script   : %s\n", t.ImageInfo.PreImage)
	fmt.Printf("PostImage script  : %s\n", t.ImageInfo.PostImage)

	if t.ClientInfo.Neighbor != "" {
		fmt.Printf("Streaming to      : %s\n", t.ClientInfo.Neighbor)
	} else {
		fmt.Println("Last client in chain, not forwarding stream")
	}
}

func reportClientStatus(status string) {
	ssurl := fmt.Sprintf("http://%s:8080/setClientStatus/%s", pusherIP, status)
	log.Printf("Reporting client status: %s", ssurl)
	ssreq, err := http.Get(ssurl)
	if err != nil || ssreq.StatusCode != 200 {
		log.Fatalf("FAILED to report status %s (%d)", status, ssreq.StatusCode)
	}
	ssreq.Body.Close()
}

func putImage() {
	// upload new image to master from file/device
	basename := filepath.Base(imageToUpload)
	url := fmt.Sprintf("http://%s:8080/saveImage/%s", pusherIP, basename)
	fmt.Printf("PUT %s\n", url)
	f, err := os.Open(imageToUpload)
	if err != nil {
		log.Fatalf("Cannot open %s: %s", imageToUpload, err)
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	request, err := http.NewRequest("PUT", url, reader)
	if err != nil {
		log.Fatalf("Cannot PUT -- server running?")
	}
	fileinfo, err := f.Stat()
	if err != nil {
		log.Fatal("Cannot stat() file")
	}
	request.ContentLength = fileinfo.Size()

	client := &http.Client{}
	response, ferr := client.Do(request)
	if ferr != nil {
		log.Fatalf("PUT FAILED: %s", err)
	} else {
		defer response.Body.Close()
		if response.StatusCode == http.StatusOK {
			fmt.Println("PUT completed successfully")
		} else {
			fmt.Printf("PUT failed with status %d (403 = File exists)\n", response.StatusCode)
		}
	}
}
