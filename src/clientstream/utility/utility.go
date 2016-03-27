package utility

// package clientstream

import (
	"../colorprint"
	"fmt"
	"github.com/fatih/color"
	"os"
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

// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// HELPER METHODS
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
// =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-

// CheckError(err error)
// --------------------------------------------------------------------------------------------
// DESCRIPTION:
// -------------------
// Prints error message into console in red
func CheckError(err error) {
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
func PrintAvSegs(segsAvail []int64) {
	colorprint.Warning("Segments available")
	for _, element := range segsAvail {
		fmt.Printf("\rProcessing segment[%d]", element)
	}
	fmt.Println()
	colorprint.Warning("XXXXXXXXXXXXXXXXX")
}
