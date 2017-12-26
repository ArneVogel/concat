package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"github.com/abiosoft/semaphore"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"flag"
	"runtime"
)

const edgecastLinkBegin string = "http://"
const edgecastLinkBaseEnd string = "index"
const edgecastLinkM3U8End string = ".m3u8"
const targetdurationStart string = "TARGETDURATION:"
const targetdurationEnd string = "\n#ID3"
const resolutionStart string = `NAME="`
const resolutionEnd string = `"`
const qualityStart string = `VIDEO="`
const qualityEnd string = `"`
const sourceQuality string = "chunked"
const chunkFileExtension string = ".ts"
const currentReleaseLink string = "https://github.com/ArneVogel/concat/releases/latest"
const currentReleaseStart string = `<a href="/ArneVogel/concat/releases/download/`
const currentReleaseEnd string = `/concat"`
const versionNumber string = "v0.2"
var ffmpegCMD string = `ffmpeg`

var debug bool
var twitch_client_id string = "aokchnui2n8q38g0vezl9hq6htzy4c"

var sem = semaphore.New(5)

/*
	Returns the signature and token from a tokenAPILink
	signature and token are needed for accessing the usher api
*/
func accessTokenAPI(tokenAPILink string) (string, string, error) {
	if debug {
		fmt.Printf("\ntokenAPILink: %s\n", tokenAPILink)
	}

	resp, err := http.Get(tokenAPILink)
	if err != nil {
		return "", "", err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	// See https://blog.golang.org/json-and-go "Decoding arbitrary data"
	var data interface{}
	err = json.Unmarshal(body, &data)
	m := data.(map[string]interface{})
	sig := fmt.Sprintf("%v", m["sig"])
	token := fmt.Sprintf("%v", m["token"])
	return sig, token, err
}

func accessUsherAPI(usherAPILink string) (map[string]string, error) {
	resp, err := http.Get(usherAPILink)
	if err != nil {
		return make(map[string]string), err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return make(map[string]string), err
	}

	respString := string(body)

	if debug {
		fmt.Printf("\nUsher API response:\n%s\n", respString)
	}

	var re = regexp.MustCompile(qualityStart+"([^\"]+)"+qualityEnd+"\n([^\n]+)\n")
	match := re.FindAllStringSubmatch(respString, -1)

	edgecastURLmap := make(map[string]string)

	for _, element := range match {
		edgecastURLmap[element[1]] = element[2]
	}

	return edgecastURLmap, err
}

func getM3U8List(m3u8Link string) (string, error) {
	resp, err := http.Get(m3u8Link)
	if err != nil {
		return "", err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), err
}

/*
	Returns the number of chunks to download based of the start and end time and the target duration of a
	chunk. Adding 1 to overshoot the end by a bit
*/
func numberOfChunks(sh int, sm int, ss int, eh int, em int, es int, target int) int {
	start_seconds := sh*3600 + sm*60 + ss
	end_seconds := eh*3600 + em*60 + es

	return ((end_seconds - start_seconds) / target) + 1
}

func startingChunk(sh int, sm int, ss int, target int) int {
	start_seconds := sh*3600 + sm*60 + ss
	return (start_seconds / target)
}

func downloadChunk(newpath string, edgecastBaseURL string, chunkNum string, chunkName string, vodID string, wg *sync.WaitGroup) {
	sem.Acquire()

	if debug {
		fmt.Printf("Downloading: %s\n", edgecastBaseURL + chunkName)
	} else {
		fmt.Print(".");
	}

	resp, err := http.Get(edgecastBaseURL + chunkName)
	if err != nil {
		os.Exit(1)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		os.Exit(1)
	}

	_ = ioutil.WriteFile(newpath + "/" + vodID+"_"+chunkNum+chunkFileExtension, body, 0644)

	defer wg.Done()
	sem.Release()
}


func ffmpegCombine(newpath string, chunkNum int, startChunk int, vodID string) {
	concat := `concat:`
	for i := startChunk; i < (startChunk + chunkNum); i++ {
		s := strconv.Itoa(i)
		concat += newpath + "/" + vodID + "_" + s + chunkFileExtension + "|"
	}
	//Remove the last "|"
	concat = concat[0 : len(concat)-1]
	concat += ``

	args := []string{"-i", concat, "-c", "copy", "-bsf:a", "aac_adtstoasc", "-fflags", "+genpts", vodID + ".mp4"}

	if debug {
		fmt.Printf("Running ffmpeg: %s %s\n", ffmpegCMD, args)
	}

	cmd := exec.Command(ffmpegCMD, args...)
	var errbuf bytes.Buffer
	cmd.Stderr = &errbuf
	err := cmd.Run()
	if err != nil {
		fmt.Println(errbuf.String())
		fmt.Println("ffmpeg error")
	}
}

func deleteChunks(newpath string, chunkNum int, startChunk int, vodID string) {
	var del string
	for i := startChunk; i < (startChunk + chunkNum); i++ {
		s := strconv.Itoa(i)
		del = newpath + "/" + vodID + "_" + s + chunkFileExtension
		err := os.Remove(del)
		if err != nil {
			fmt.Println("Could not delete all chunks, try manually deleting them", err)
		}
	}
}

func printQualityOptions(vodIDString string) {
	vodID, _ := strconv.Atoi(vodIDString)

	tokenAPILink := fmt.Sprintf("http://api.twitch.tv/api/vods/%v/access_token?&client_id="+twitch_client_id, vodID)

	fmt.Println("Contacting Twitch Server")

	sig, token, err := accessTokenAPI(tokenAPILink)
	if err != nil {
		fmt.Println("Couldn't access twitch token api")
		os.Exit(1)
	}

	usherAPILink := fmt.Sprintf("http://usher.twitch.tv/vod/%v?nauthsig=%v&nauth=%v&allow_source=true", vodID, sig, token)


	resp, err := http.Get(usherAPILink)
	if err != nil {
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	respString := string(body)

	qualityCount := strings.Count(respString, resolutionStart)
	for i := 0; i < qualityCount; i++ {
		rs := strings.Index(respString, resolutionStart) + len(resolutionStart)
		re := strings.Index(respString[rs:len(respString)], resolutionEnd) + rs
		qs := strings.Index(respString, qualityStart) + len(qualityStart)
		qe := strings.Index(respString[qs:len(respString)], qualityEnd) + qs
		if (strings.Contains(respString[rs:re], "p60") || strings.Contains(respString[rs:re], "p30") ) {
			fmt.Printf("resolution: %s, download with -quality=\"%s\"\n", respString[rs:re], respString[qs:qe])
		} else {
			fmt.Printf("resolution: %s30, download with -quality=\"%s\"\n", respString[rs:re], respString[qs:qe])
		}

		respString = respString[qe:len(respString)]
	}
}

func wrongInputNotification() {
	fmt.Println("Call the program with -help for information on how to use it :^)")
}

func downloadPartVOD(vodIDString string, start string, end string, quality string) {
	var vodID, vodSH, vodSM, vodSS, vodEH, vodEM, vodES int

	vodID, _ = strconv.Atoi(vodIDString)

	if end != "full" {
		startArray := strings.Split(start, " ")
		endArray := strings.Split(end, " ")

		vodSH, _ = strconv.Atoi(startArray[0]) //start Hour
		vodSM, _ = strconv.Atoi(startArray[1]) //start minute
		vodSS, _ = strconv.Atoi(startArray[2]) //start second
		vodEH, _ = strconv.Atoi(endArray[0]) //end hour
		vodEM, _ = strconv.Atoi(endArray[1]) //end minute
		vodES, _ = strconv.Atoi(endArray[2]) //end second

		if (vodSH*3600 + vodSM*60 + vodSS) > (vodEH*3600 + vodEM*60 + vodES) {
			wrongInputNotification()
		}
	}

	_, err := os.Stat(vodIDString + ".mp4")

	if ( err == nil || !os.IsNotExist(err)) {
		fmt.Printf("Destination file %s already exists!\n", vodIDString + ".mp4")
		os.Exit(1)
	}

	tokenAPILink := fmt.Sprintf("http://api.twitch.tv/api/vods/%v/access_token?&client_id="+twitch_client_id, vodID)

	fmt.Println("Contacting Twitch Server")

	sig, token, err := accessTokenAPI(tokenAPILink)
	if err != nil {
		fmt.Println("Couldn't access twitch token api")
		os.Exit(1)
	}

	if debug {
		fmt.Printf("\nSig: %s, Token: %s\n", sig, token)
	}

	usherAPILink := fmt.Sprintf("http://usher.twitch.tv/vod/%v?nauthsig=%v&nauth=%v&allow_source=true", vodID, sig, token)

	if debug {
		fmt.Printf("\nusherAPILink: %s\n", usherAPILink)
	}

	edgecastURLmap, err := accessUsherAPI(usherAPILink)
	if err != nil {
		fmt.Println("Count't access usher api")
		os.Exit(1)
	}

	if debug {
		fmt.Println(edgecastURLmap)
	}

	// I don't see what this does. With this you can't download in source quality (chunked).
	// Fixed. But "chunked" playlist not always available, have to loop and find max quality manually

	m3u8Link, ok := edgecastURLmap[quality]

	if ok {
		fmt.Printf("Selected quality: %s\n", quality)
	} else {
		fmt.Printf("Couldn't find quality: %s\n", quality)

		// Try to find source quality playlist
		if quality != sourceQuality {
			quality = sourceQuality

			m3u8Link, ok = edgecastURLmap[quality]
		}

		if ok {
			fmt.Printf("Downloading in source quality: %s\n", quality)
		} else {
			// Quality still not matched
			resolution_max := 0
			fps_max := 0
			resolution_tmp := 0
			fps_tmp := 0
			var key_tmp []string

			// Find max quality
			for key, _ := range edgecastURLmap {
				key_tmp = strings.Split(key, "p")

				resolution_tmp, _ = strconv.Atoi(key_tmp[0])

				if len(key_tmp) > 1 {
					fps_tmp, _ = strconv.Atoi(key_tmp[1])
				} else {
					fps_tmp = 0
				}

				if ( resolution_tmp > resolution_max || resolution_tmp == resolution_max && fps_tmp > fps_max ) {
					quality = key
					fps_max = fps_tmp
					resolution_max = resolution_tmp
				}
			}

			m3u8Link, ok = edgecastURLmap[quality]

			if ok {
				fmt.Printf("Downloading in max available quality: %s\n", quality)
			} else {
				fmt.Println("No available quality options found")
				os.Exit(1)
			}
		}
	}

	edgecastBaseURL := m3u8Link
	edgecastBaseURL = edgecastBaseURL[0 : strings.Index(edgecastBaseURL, edgecastLinkBaseEnd)]

	if debug {
		fmt.Printf("\nedgecastBaseURL: %s\nm3u8Link: %s\n", edgecastBaseURL, m3u8Link)
	}

	fmt.Println("Getting Video info")

	m3u8List, err := getM3U8List(m3u8Link)
	if err != nil {
		fmt.Println("Couldn't download m3u8 list")
		os.Exit(1)
	}

	if debug {
		fmt.Printf("\nm3u8List:\n%s\n", m3u8List)
	}

	var re = regexp.MustCompile("\n([^#]+)\n")
	match := re.FindAllStringSubmatch(m3u8List, -1)

	var m3u8Array []string

	for _, element := range match {
		m3u8Array = append(m3u8Array, element[1])
	}

	if debug {
		fmt.Printf("\nItems list: %v\n", m3u8Array)
	}

	var chunkNum, startChunk int

	if end != "full" {
		targetduration, _ := strconv.Atoi(m3u8List[strings.Index(m3u8List, targetdurationStart)+len(targetdurationStart) : strings.Index(m3u8List, targetdurationEnd)])
		chunkNum = numberOfChunks(vodSH, vodSM, vodSS, vodEH, vodEM, vodES, targetduration)
		startChunk = startingChunk(vodSH, vodSM, vodSS, targetduration)
	} else {
		fmt.Println("Dowbloading full vod")

		chunkNum = len(m3u8Array)
		startChunk = 0
	}

	if debug {
		fmt.Printf("\nchunkNum: %v\nstartChunk: %v\n", chunkNum, startChunk)
	}

	var wg sync.WaitGroup
	wg.Add(chunkNum)

	newpath := filepath.Join(".", "_" + vodIDString)

	err = os.MkdirAll(newpath, os.ModePerm)
	if err != nil {
		fmt.Println("Count't create directory")
		os.Exit(1)
	}
	fmt.Printf("Created temp dir: %s\n", newpath)

	fmt.Println("Starting Download")

	for i := startChunk; i < (startChunk + chunkNum); i++ {

		s := strconv.Itoa(i)
		n := m3u8Array[i]
		go downloadChunk(newpath, edgecastBaseURL, s, n, vodIDString, &wg)
	}
	wg.Wait()

	fmt.Println("\nCombining parts")

	ffmpegCombine(newpath, chunkNum, startChunk, vodIDString)

	fmt.Println("Deleting chunks")

	deleteChunks(newpath, chunkNum, startChunk, vodIDString)

	fmt.Println("Deleting temp dir")

	os.Remove(newpath)

	fmt.Println("All done!")
}

func rightVersion() bool {
	resp, err := http.Get(currentReleaseLink)
	if err != nil {
		fmt.Println("Couldn't access github while checking for most recent release.")
	}

	body, _ := ioutil.ReadAll(resp.Body)

	respString := string(body)

	cs := strings.Index(respString, currentReleaseStart) + len(currentReleaseStart)
	ce := cs + len(versionNumber)
	return respString[cs:ce] == versionNumber
}

func init() {
	if runtime.GOOS == "windows" {
	    ffmpegCMD = `ffmpeg.exe`
	}
}

func main() {

	qualityInfo := flag.Bool("qualityinfo", false, "if you want to see the avaliable quality options")

	standardStartAndEnd := "HH MM SS"
	standardVOD := "123456789"
	vodID := flag.String("vod", standardVOD, "the vod id https://www.twitch.tv/videos/123456789")
	start := flag.String("start", standardStartAndEnd, "For example: 0 0 0 for starting at the bedinning of the vod")
	end := flag.String("end", standardStartAndEnd, "For example: 1 20 0 for ending the vod at 1 hour and 20 minutes")
	quality := flag.String("quality", sourceQuality, "chunked for source quality is automatically used if -quality isn't set")
	debugFlag := flag.Bool("debug", false, "debug output")

	flag.Parse()

	debug = *debugFlag;

	if !rightVersion() {
		fmt.Printf("You are using an old version of concat. Check out %s for the most recent version.\n\n",currentReleaseLink)
	}

	if *vodID == standardVOD {
		wrongInputNotification()
		os.Exit(1)
	}

	if *qualityInfo {
		printQualityOptions(*vodID)
		os.Exit(1)
	}

	if (*start != standardStartAndEnd && *end != standardStartAndEnd) {
		downloadPartVOD(*vodID, *start, *end, *quality);
	} else {
		downloadPartVOD(*vodID, "0", "full", *quality);
	}

	os.Exit(1)
}
