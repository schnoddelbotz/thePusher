package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	IMG_DMG              = "DMG" // for mac
	IMG_DDIMG            = "IMG" // for linux
	IMG_TAR              = "TAR" // for any
	COMP_GZIP            = "GZ"
	COMP_BZIP2           = "BZ2"
	COMP_NONE            = "NONE"
	STATUS_NONE          = "NONE"
	STATUS_READY_WAITING = "WAIT"
	STATUS_BUSY          = "BUSY"
	STATUS_DONE_OK       = "DONE"
	STATUS_ERROR         = "ERROR"
)

type ClientTask struct {
	ClientInfo ClientInfo `json:"client"`
	ImageInfo  Image      `json:"image"`
}

type ClientInfo struct {
	Address  string
	Group    string
	Image    string
	Neighbor string
	Status   string
}

type asset_info struct {
	ContentType string
	IsGzipped   bool
}

var clients = map[string]ClientInfo{}
var masterConfig Config
var mutex = &sync.Mutex{}

var assetMap = map[string]asset_info{
	"index.html":      {"text/html", false},
	"favicon.png":     {"image/png", false},
	"crossdomain.xml": {"text/xml", false},
	"assets/app.js":   {"application/javascript", false},
	"assets/app.css":  {"text/css", false},
}

var serverURI string
var listenAddr string
var wsserver *Server

func runWebserver() {
	initClients()
	serverPort := "8080"
	serverIP := "127.0.0.1"
	var serverProto = "http"
	var serverPortInt = 8080
	var err error

	if !strings.Contains(listenAddr, ":") {
		log.Fatal("Listen address must be of format [IP]:Port")
	}
	_tmp := strings.Split(listenAddr, ":")
	serverPort = _tmp[1]
	if serverPortInt, err = strconv.Atoi(serverPort); err != nil {
		log.Fatal("Invalid port for --listen argument")
	}
	if _tmp[0] != "" {
		serverIP = _tmp[0]
	}
	log.Printf("About to listen on %s ...", listenAddr)
	serverURI = fmt.Sprintf("%s://%s:%d", serverProto, serverIP, serverPortInt)
	log.Printf("Go to %s://%s:%d/", serverProto, serverIP, serverPortInt)

	// built-in asset-based routes (static content)
	http.HandleFunc("/", assetHttpHandler)
	http.HandleFunc("/index.html", assetHttpHandler)
	http.HandleFunc("/favicon.png", assetHttpHandler)
	http.HandleFunc("/assets/", assetHttpHandler) // app.js / app.css
	// web UI
	http.HandleFunc("/getClientGroups.json", clientGroupsHandler) // web UI retrieves all client groups
	http.HandleFunc("/getClientStati.json", clientStatiHandler)
	// status update websocket
	wsserver = NewWebSocketServer("/websocket")
	go wsserver.Listen()

	// client api
	http.HandleFunc("/getClientTask", clientTaskHandler)      // clients call it to receive their task
	http.HandleFunc("/setClientStatus/", clientStatusHandler) // clients submit their status
	http.HandleFunc("/startStream/", startStreamHandler)      // last client in chain calls us...
	// retrieving new images (thePusher putImage)
	http.HandleFunc("/saveImage/", saveImageHandler) // clients can PUT new images for later restore
	// serve /static user-content
	if staticContentRoot != "" {
		files := http.FileServer(http.Dir(staticContentRoot))
		http.Handle("/static/", http.StripPrefix("/static/", files))
	}

	err = http.ListenAndServe(listenAddr, nil)
	if err != nil {
		log.Fatal(err)
	}
}

func initClients() {
	// sets global clients map for quick lookup...
	readConfig(imageStorage + "/thePusher-config.hcl")
	for _, group := range masterConfig.Clientgroups {
		hostCount := len(group.Hosts)
		for hostIndex, hostname := range group.Hosts {
			var clientEntry ClientInfo
			clientEntry.Status = STATUS_NONE
			clientEntry.Image = group.Image
			clientEntry.Group = group.Name
			clientEntry.Address = hostname
			if hostIndex < hostCount-1 {
				clientEntry.Neighbor = group.Hosts[hostIndex+1]
			}
			clients[hostname] = clientEntry
		}
	}
	//log.Print(clients)
}

func clientTaskHandler(w http.ResponseWriter, request *http.Request) {
	var task ClientTask
	responseCode := 200
	clientIP, _, _ := net.SplitHostPort(request.RemoteAddr)
	cinfo, ok := clients[clientIP]
	if ok {
		task.ClientInfo = cinfo
		task.ImageInfo = getImageByKey(cinfo.Image)
		myJSON, _ := json.Marshal(task)
		w.Write(myJSON)

		// lock!
		mutex.Lock()
		c := clients[clientIP]
		c.Status = STATUS_READY_WAITING
		clients[clientIP] = c
		mutex.Unlock()
		wsserver.sendAll(&c)
	} else {
		responseCode = 404
		http.NotFound(w, request)
	}

	if verbose {
		log.Printf("%s %3d %s %s", request.RemoteAddr, responseCode, request.Method, request.URL.Path)
	}
}

func clientStatusHandler(w http.ResponseWriter, request *http.Request) {
	// update a single client's status
	responseCode := 200
	clientIP, _, _ := net.SplitHostPort(request.RemoteAddr)
	_, ok := clients[clientIP]
	if ok {
		var newStatus string
		uriSegments := strings.Split(request.RequestURI, "/")
		cStatus := uriSegments[2]
		switch cStatus {
		case STATUS_BUSY:
			newStatus = STATUS_BUSY
		case STATUS_DONE_OK:
			newStatus = STATUS_DONE_OK
		default:
			newStatus = STATUS_ERROR
		}
		// lock!
		mutex.Lock()
		c := clients[clientIP]
		c.Status = newStatus
		clients[clientIP] = c
		mutex.Unlock()
		wsserver.sendAll(&c)
	} else {
		responseCode = 404
		http.NotFound(w, request)
	}

	if verbose {
		log.Printf("%s %3d %s %s", request.RemoteAddr, responseCode, request.Method, request.URL.Path)
	}
}

func startStreamHandler(w http.ResponseWriter, request *http.Request) {
	uriSegments := strings.Split(request.RequestURI, "/")
	groupName := uriSegments[2]
	group := getClientgroupByKey(groupName)
	image := getImageByKey(group.Image)
	if group.Name != "" {
		firstHost := group.Hosts[0]
		log.Printf("Streaming %s (%s) to group %s (first: %s)", image.Name, image.Filename, groupName, firstHost)
		go startStream(image.Filename, firstHost, group)
	}
}

func startStream(file string, target string, cgroup Clientgroup) {
	url := fmt.Sprintf("http://%s:8080/receiveImage", target)
	client := &http.Client{}

	// as "last" client in chain isn't guaranteed to
	// be the last client booted, this must WAIT HERE until
	// all clients of group reported ready state
	ready := false
	cgroupHostCount := len(cgroup.Hosts)
	for !ready {
		waitingCount := 0
		for _, hostname := range cgroup.Hosts {
			host := clients[hostname]
			if host.Status == STATUS_NONE {
				log.Printf("--  waiting for %s (still has status %s)\n", hostname, host.Status)
			}
			if host.Status == STATUS_READY_WAITING {
				waitingCount = waitingCount + 1
			}
		}
		if waitingCount == cgroupHostCount {
			log.Printf("Streaming to group %s finally starts now, all hosts ready!", cgroup.Name)
			ready = true
		} else {
			log.Print("Not all hosts ready yet, still waiting...")
			time.Sleep(5 * time.Second)
		}
	}

	filepath := fmt.Sprintf("%s/%s", imageStorage, file)
	fileHandle, err := os.Open(filepath)
	if err != nil {
		log.Fatalf("Cannot open %s: %s", file, err)
	}
	fileInfo, _ := fileHandle.Stat()
	imageReader := bufio.NewReader(fileHandle)
	request, err := http.NewRequest("PUT", url, imageReader)

	// FIXME: should write progress so websocket can deliver and web UI can show it
	request.ContentLength = fileInfo.Size()
	response, err := client.Do(request)
	if err != nil {
		log.Fatal(err)
	} else {
		defer response.Body.Close()
		_, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Fatal(err)
		}
	}
	fileHandle.Close()
	log.Printf("Streaming %s completed", file)
}

// asset web server (for web-frontend)

func assetHttpHandler(w http.ResponseWriter, request *http.Request) {
	// serve static / compiled-in assets (using go-bindata)
	requestPath := request.URL.Path[1:]
	responseCode := http.StatusOK
	if requestPath == "" {
		requestPath = "index.html"
	}
	info, ok := assetMap[requestPath]
	if ok {
		gzipExtension := ""
		w.Header().Set("Content-Type", info.ContentType)
		if info.IsGzipped {
			w.Header().Set("Content-Encoding", "gzip")
			gzipExtension = ".gz"
		}
		data, _ := Asset(requestPath + gzipExtension)
		w.Write(data)
	} else {
		responseCode = 404
		http.NotFound(w, request)
	}

	if verbose {
		log.Printf("%s %3d %s %s", request.RemoteAddr, responseCode, request.Method, request.URL.Path)
	}
}

func clientGroupsHandler(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	myJSON, _ := json.Marshal(masterConfig.Clientgroups)
	w.Write(myJSON)
}

func clientStatiHandler(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	myJSON, _ := json.Marshal(clients)
	w.Write(myJSON)
}

func saveImageHandler(w http.ResponseWriter, request *http.Request) {
	// test: curl --upload-file my.img  http://localhost:8080/saveImage/my.img
	log.Printf("/saveImage starting (source: %s)", request.RemoteAddr)
	uriSegments := strings.Split(request.RequestURI, "/")
	filename := uriSegments[2]
	filepath := fmt.Sprintf("%s/%s", imageStorage, filename)
	if _, err := os.Stat(filepath); err == nil {
		log.Printf("REFUSED %s: File exists", filename)
		http.Error(w, "Forbidden (File exists)", http.StatusForbidden)
		return
	}
	// FIXME: add "clientsAllowedPut = ["a.b.c.d","e.f.g.h"]" and verify here [empty to forbid any]
	outfile, err := os.Create(filepath)
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}
	defer outfile.Close()
	outfileWriter := bufio.NewWriter(outfile)
	reader := bufio.NewReader(request.Body)
	reader.WriteTo(outfileWriter)
	outfileWriter.Flush()
	outfile.Sync()
	log.Print("/saveImage completed")
}
