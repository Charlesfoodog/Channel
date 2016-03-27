package main

// package clientstream

import (
	"./colorprint"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fatih/color"
	"io/ioutil"
	"log"
	"net"
	"net/rpc"
	"os"
	"regexp"
	"strconv"
	"sync"
)

// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
//  STRUCTS & TYPES
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-

// --> VidSegment <---
// -------------------
// DESCRIPTION:
// -------------------
// This struct holds a particular part of a video file. Id refers to the segment id and the body refers to the bytecount of the actual video bytes
type VidSegment struct {
	Id   int
	Body []byte
}

type ReqStruct struct {
	Filename  string
	SegmentId int
}

// --> Video <---
// --------------
// DESCRIPTION:
// -------------------
// This struct holds the VidSegments of a particular video file. SegNums refers to the total number of segments in the entire video.
// This colorprint.Info is used to reorder the segments when playing the video stream.
type Video struct {
	Name      string
	SegNums   int64
	SegsAvail []int64
	Segments  map[int]VidSegment
}

// --> FileSys <---
// ----------------
// DESCRIPTION
// -------------------
// This struct represents the local FileSystem to hold the Video's. Each FileSys object has an id and a map of Files with the keys to the map being the
// actual filename.
// This colorprint.Info is used to check if a node actually has the file
type FileSys struct {
	Id    int
	Files map[string]Video
}

// --> File <---
// ----------------
// DESCRIPTION
// -------------------
// This struct represents the Files as their names and the directory that they are located in
type File struct {
	Name string `json:"name"`
	Path string `json:"dir"`
}

// --> FilePath <---
// ----------------
// DESCRIPTION
// -------------------
// This struct represents the filenames and their directory paths from which we are to read the files to be processed
type FilePath struct {
	Files []File `json:"Files"`
}

// --> Response <---
// -----------------
// DESCRIPTION:
// -------------------
// This struct represents the response that an RPC call will write to. It is used to check if a node has a particular file and if it does, which parts
// of that file it has in its local filesystem.
type Response struct {
	Avail     bool
	SegNums   int64
	SegsAvail []int64
}

// --> Service <---
// ----------------
// DESCRIPTION:
// -------------------
// This type just holds an integer to use for registering the RPC Service
type Service int

// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// GLOBAL VARS
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-

var localFileSys FileSys
var fileSysLock *sync.RWMutex
var bytecount int = 2048

// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// INBOUND RPC CALL METHODS
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-

// (service *Service) localFileAvailability(filename string, response *Response) error
// -----------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// This method responds to an rpc Call for a particular segment of a file. It first looks up and checks if the video file is available. If
// the file is available, it continues on to see if the segment is available. If the segment is available, it returns a response with the VidSegment.
// In case of unavailability, it will either return an error saying "File Unavailable." or "Segment unavailable." depending on what was unavailable.
// The method locks the local filesystem for all Reads and Writes during the process.
func (service *Service) LocalFileAvailability(filename string, response *Response) error {
	colorprint.Debug("INBOUND RPC REQUEST: Checking File Availability " + filename)
	fileSysLock.RLock()
	video, ok := localFileSys.Files[filename]
	if ok {
		colorprint.Info("File " + filename + " is available")
		colorprint.Debug("INBOUND RPC REQUEST COMPLETED")
		response.Avail = true
		response.SegNums = video.SegNums
		response.SegsAvail = video.SegsAvail
	} else {
		colorprint.Alert("File " + filename + " is unavailable")
		response.Avail = false
	}
	fileSysLock.RUnlock()
	return nil
}

// (service *Service) sendFileSegment(filename string, segment *VidSegment) error <--
// ----------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// This method responds to an rpc Call for a particular segment of a file. It first looks up and checks if the video file is available. If the
// file is available, it continues on to see if the segment is available. If the segment is available, it returns a response with the VidSegment.
// In case of unavailability, it will either return an error saying "File Unavailable." or "Segment unavailable." depending on what was unavailable.
// The method locks the local filesystem for all Reads and Writes during the process.
func (service *Service) GetFileSegment(segReq *ReqStruct, segment *VidSegment) error {
	colorprint.Debug("INBOUND RPC REQUEST: Sending video segment for " + segReq.Filename)
	var seg VidSegment
	fileSysLock.RLock()
	outputstr := ""
	video, ok := localFileSys.Files[segReq.Filename]
	if ok {
		outputstr += ("\nNode is asking for segment no. " + strconv.Itoa(segReq.SegmentId) + " for " + segReq.Filename)
		_, ok := (video.Segments[1])
		if ok {
			outputstr += ("\nSeg 1 available")
		}
		seg, ok = video.Segments[segReq.SegmentId]
		if ok {
			segment.Body = seg.Body
		} else {
			outputstr += ("\nSegment " + strconv.Itoa(segReq.SegmentId) + " unavailable for " + segReq.Filename)
			return errors.New("Segment unavailable.")
			fileSysLock.Unlock()
		}
	} else {

		return errors.New("File unavailable.")
		fileSysLock.Unlock()
	}
	colorprint.Warning(outputstr)
	return nil
}

// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// OUTBOUND RPC CALL METHODS
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-

// checkFileAvailability(filename string, nodeService *rpc.Client) (bool, int64, map[int]int64)
// --------------------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// This method calls an RPC method to another node to check if they have a video available. If it is available on the node,
// if no errors occur in the Call, the method checks the response to see if the file is available. If it is, it reads the response
// to obtain the map of segments and the total number of segments of the file
func checkFileAvailability(filename string, nodeadd string, nodeService *rpc.Client) (bool, int64, []int64) {
	colorprint.Debug("OUTBOUND REQUEST: Check File Availability")
	var response Response
	var segNums int64
	var segsAvail []int64
	err := nodeService.Call("Service.LocalFileAvailability", filename, &response)
	checkError(err)
	colorprint.Debug("OUTBOUND REQUEST COMPLETED")
	if response.Avail == true {
		fmt.Println("File:", filename, " is available")
		segNums = response.SegNums
		segsAvail = response.SegsAvail
		return true, segNums, segsAvail
	} else {
		fmt.Println("File:", filename, " is not available on node["+""+"].")
		return false, 0, nil
	}
}

// getVideoSegment(filename string, segId int, nodeService *rpc.Client) (bool, int64, map[int]int64)
// --------------------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// This method calls an RPC method to another node to obtain a particular segment of a video
func getVideoSegment(fname string, segId int, nodeService *rpc.Client) VidSegment {
	segReq := &ReqStruct{
		Filename:  fname,
		SegmentId: segId,
	}
	var vidSeg VidSegment
	err := nodeService.Call("Service.GetFileSegment", segReq, &vidSeg)
	checkError(err)
	return vidSeg
}

// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// OTHER METHODS
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-

// saveSegToFileSys()
// --------------------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// This method calls an RPC method to another node to obtain a particular segment of a video
func saveSegToFileSys(service *rpc.Client, segNums int64, fname string) {
	var newVid Video
	vidMap := make(map[int]VidSegment)
	var segsAvail []int64
	for i := 1; i <= int(segNums); i++ {
		colorprint.Warning("Asking for segment " + strconv.Itoa(int(i)))
		vidSeg := getVideoSegment(fname, i, service)
		vidMap[i] = vidSeg
		segsAvail = append(segsAvail, int64(i))
	}
	newVid = Video{
		Name:      fname,
		SegNums:   segNums,
		SegsAvail: segsAvail,
		Segments:  vidMap,
	}
	fileSysLock.Lock()
	localFileSys.Files[fname] = newVid
	fileSysLock.Unlock()
	fmt.Println()
	colorprint.Info(fname + " saved into file system.")
	colorprint.Warning("Saving file info into local json list")
}

// processLocalVideosIntoFileSys()
// --------------------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// This method loads up a local json file to see which files are available in the local file system. Once
// the read has been completed, the files are then processed into the FileSys map accordingly
func processLocalVideosIntoFileSys() {
	locFiles, err := ioutil.ReadFile("./localFiles.json")
	checkError(err)
	files := make([]File, 0)
	var filePaths FilePath
	filePaths.Files = files
	err = json.Unmarshal(locFiles, &filePaths)
	checkError(err)
	// Initialize local file system
	localFileSys = FileSys{
		Id:    1,
		Files: make(map[string]Video),
	}
	for _, value := range filePaths.Files {
		colorprint.Info("Processing " + value.Name + " at " + value.Path)
		dat, err := ioutil.ReadFile(value.Path)
		checkError(err)
		colorprint.Info("---------------------------------------------------------------------------")
		colorprint.Info("Video:" + value.Name + " has " + strconv.Itoa(len(dat)/bytecount) + " segments.")
		segsAvail, vidMap := convByteArrayToSeg(dat)

		vid := Video{
			Name:      value.Name,
			SegNums:   int64(len(dat) / bytecount),
			SegsAvail: segsAvail,
			Segments:  vidMap,
		}
		printAvSegs(segsAvail)
		fileSysLock.Lock()
		localFileSys.Files[value.Name] = vid
		fileSysLock.Unlock()
		colorprint.Info("Completed Processing " + value.Name + " at " + value.Path)
		colorprint.Info("---------------------------------------------------------------------------")
	}

}

// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// HELPER METHODS
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-

// checkError(err error)
// --------------------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// Prints error message into console in red
func checkError(err error) {
	if err != nil {
		color.Set(color.FgRed)
		fmt.Println(err)
		color.Unset()
		os.Exit(-1)
	}
}

// printAvSegs(segsAvail []int64)
// --------------------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// Prints list of ids for available segment nums
func printAvSegs(segsAvail []int64) {
	colorprint.Warning("Segments available")
	for _, element := range segsAvail {
		fmt.Printf("\rProcessing segment[%d]", element)
	}
	fmt.Println()
	colorprint.Warning("XXXXXXXXXXXXXXXXX")
}

// validIP(ipAddress string, field string) bool
// --------------------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// Checks if the ip provided is valid. Accepts only the port as well eg. :3000 although in this case
// it assumes the localhost ip address
func validIP(ipAddress string, field string) bool {
	re, _ := regexp.Compile(`[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\:[0-9]{1,5}|\:[0-9]{1,5}`)
	if re.MatchString(ipAddress) {
		return true
	}
	fmt.Println("\x1b[31;1mError: "+field+":"+ipAddress, "is not in the correct format\x1b[0m")
	return false
}

// convByteArrayToSeg(bytes []byte) ([]int64, map[int]VidSegment)
// --------------------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// Converts the byte array from a video files into Video Segments.
func convByteArrayToSeg(bytes []byte) ([]int64, map[int]VidSegment) {
	vidmap := make(map[int]VidSegment)
	var segsAvail []int64
	var vidSeg VidSegment
	var eightBSeg []byte
	counter, counter2, counter3 := 1, 1, 1
	progstr := "="
	blen := len(bytes)
	altc := (blen / 100)
	for index, element := range bytes {
		eightBSeg = append(eightBSeg, element)
		if counter == bytecount {
			counter = 0
			vidSeg = VidSegment{
				Id:   ((index % bytecount) + 1),
				Body: eightBSeg,
			}
			vidmap[((index / bytecount) + 1)] = vidSeg
			segsAvail = append(segsAvail, int64(((index / bytecount) + 1)))
			eightBSeg = []byte{}
		}
		counter++
		counter2++
		counter3++
		if counter2 == altc {
			progstr += "="
			fmt.Printf("\r[%s]  - %d%%", progstr, ((counter3*100)/blen + 1))
			counter2 = 0
		}
	}
	fmt.Println()
	colorprint.Debug("SEGMENTS PROCESSED: " + strconv.Itoa((len(segsAvail))))
	return segsAvail, vidmap
}

// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// CONNECTION METHODS
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-

// setUpRPC(nodeRPC string)
// --------------------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// This method sets up the RPC connection using UDP
func setUpRPC(nodeRPC string) {
	rpcServ := new(Service)
	rpc.Register(rpcServ)
	l, e := net.Listen("tcp", nodeRPC)

	if e != nil {
		log.Fatal("listen error:", e)
	}
	colorprint.Blue("Listening on " + nodeRPC + " for incoming RPC calls")
	for i := 0; i >= 0; i++ {
		conn, _ := l.Accept()
		colorprint.Alert("=========================================================================================")
		colorprint.Debug("REQ " + strconv.Itoa(i) + ": ESTABLISHING RPC REQUEST CONNECTION WITH " + conn.LocalAddr().String())
		go rpc.ServeConn(conn)
		colorprint.Blue("REQ " + strconv.Itoa(i) + ": Request Served")
		colorprint.Alert("=========================================================================================")
	}

}

// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// IO METHODS
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-

// instr(nodeRPC string, nodeUDP string)
// --------------------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// This method responds to the user input requests
func instr(nodeRPC string, nodeUDP string) {
	var input string
	var fname string
	for i := 0; i >= 0; i++ {
		colorprint.Info(">>>> Please type in the command")
		fmt.Scan(&input)
		cmd := input
		if input == "get" {
			getHelper(nodeRPC, nodeUDP, input, fname, cmd)
		}
	}
}

// getHelper(nodeRPC string, nodeUDP string)
// --------------------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// This method responds to the user input request for "get"
func getHelper(nodeRPC string, nodeUDP string, input string, fname string, cmd string) {

	colorprint.Info(">>>> Please enter the name of the file that you would like to obtain")
	fmt.Scan(&fname)
	colorprint.Debug("<<<< " + fname)
	colorprint.Info(">>>> Please enter the address of the node you want to connect to")
	fmt.Scan(&input)
	colorprint.Debug("<<<< " + input)
	nodeAddr := input
	service, err := rpc.Dial("tcp", nodeAddr) // Connect to Service via RPC // returns *Client, err
	checkError(err)
	avail, segNums, _ := checkFileAvailability(fname, nodeAddr, service)
	if avail && (cmd == "get") {
		colorprint.Info(">>>> Would you like to get the file from the node[" + nodeRPC + "]?(y/n)")
		fmt.Scan(&input)
		colorprint.Debug("<<<< " + input)
		if input == "y" {
			saveSegToFileSys(service, segNums, fname)
		}
	}
}

// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// MAIN METHODS
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-

func main() {
	// ========================================
	fileSysLock = &sync.RWMutex{}
	processLocalVideosIntoFileSys()
	// ========================================
	if len(os.Args) == 3 {
		nodeRPC := os.Args[1]
		nodeUDP := os.Args[2]
		if !validIP(nodeRPC, "[node RPC ip:port]") || !validIP(nodeUDP, "[node UDP ip:port]") {
			os.Exit(-1)
		}

		go setUpRPC(nodeRPC)
		instr(nodeRPC, nodeUDP)

	}
}
