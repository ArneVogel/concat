package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/abiosoft/semaphore"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

const edgecastLinkBegin string = "http://"
const edgecastLinkBaseEnd string = "index"
const edgecastLinkM3U8End string = ".m3u8"
const targetdurationStart string = "TARGETDURATION:"
const targetdurationEnd string = "\n#ID3"
const ffmpegCMD string = `ffmpeg.exe`

var sem = semaphore.New(5)

/*
	Returns the signature and token from a tokenAPILink
	signature and token are needed for accessing the usher api
*/
func accessTokenAPI(tokenAPILink string) (string, string, error) {
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

func accessUsherAPI(usherAPILink string) (string, string, error) {
	resp, err := http.Get(usherAPILink)
	if err != nil {
		return "", "", err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	respString := string(body)

	m3u8Link := respString[strings.Index(respString, edgecastLinkBegin) : strings.Index(respString, edgecastLinkM3U8End)+len(edgecastLinkM3U8End)]
	edgecastBaseURL := respString[strings.Index(respString, edgecastLinkBegin):strings.Index(respString, edgecastLinkBaseEnd)]

	return edgecastBaseURL, m3u8Link, err
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

func downloadChunk(edgecastBaseURL string, chunkNum string, vodID string, wg *sync.WaitGroup) {
	sem.Acquire()
	resp, err := http.Get(edgecastBaseURL + chunkNum + ".ts")
	if err != nil {
		os.Exit(1)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		os.Exit(1)
	}

	_ = ioutil.WriteFile(vodID+"_"+chunkNum+".mp4", body, 0644)

	defer wg.Done()
	sem.Release()
}

/*


 */
func ffmpegCombine(chunkNum int, startChunk int, vodID string) {
	concat := `concat:`
	for i := startChunk; i < (startChunk + chunkNum); i++ {
		s := strconv.Itoa(i)
		concat += vodID + "_" + s + ".mp4|"
	}
	//Remove the last "|"
	concat = concat[0 : len(concat)-1]
	concat += ``

	args := []string{"-i", concat, "-c", "copy", "-bsf:a", "aac_adtstoasc", "-fflags", "+genpts", vodID + ".mp4"}

	cmd := exec.Command(ffmpegCMD, args...)
	var errbuf bytes.Buffer
	cmd.Stderr = &errbuf
	err := cmd.Run()
	if err != nil {
		fmt.Println(errbuf.String())
		fmt.Println("ffmpeg error")
	}
}

func deleteChunks(chunkNum int, startChunk int, vodID string) {
	var del string
	for i := startChunk; i < (startChunk + chunkNum); i++ {
		s := strconv.Itoa(i)
		del = vodID + "_" + s + ".mp4"
		err := os.Remove(del)
		if err != nil {
			fmt.Println("could not delete all chunks, try manually deleting them", err)
		}
	}
}

func wrongInputNotification() {
	fmt.Println("Call the program with the vod id, start and end time following: concat.exe VODID HH MM SS HH MM SS\nwhere VODID is the number you see in the url of the vod (https://www.twitch.tv/videos/123456789 => 123456789) the first HH MM SS is the start time and the second HH MM SS is the end time.\nSo downloading the first one and a half hours of a vod would be: concat.exe 123456789 0 0 0 1 30 0")
	os.Exit(1)
}

func main() {
	var vodID, vodSH, vodSM, vodSS, vodEH, vodEM, vodES int
	var vodIDString string
	if len(os.Args) >= 8 {
		vodIDString = os.Args[1]
		vodID, _ = strconv.Atoi(os.Args[1])
		vodSH, _ = strconv.Atoi(os.Args[2]) //start Hour
		vodSM, _ = strconv.Atoi(os.Args[3]) //start minute
		vodSS, _ = strconv.Atoi(os.Args[4]) //start second
		vodEH, _ = strconv.Atoi(os.Args[5]) //end hour
		vodEM, _ = strconv.Atoi(os.Args[6]) //end minute
		vodES, _ = strconv.Atoi(os.Args[7]) //end second
	} else {
		wrongInputNotification()
	}

	if (vodSH*3600 + vodSM*60 + vodSS) > (vodEH*3600 + vodEM*60 + vodES) {
		wrongInputNotification()
	}

	tokenAPILink := fmt.Sprintf("http://api.twitch.tv/api/vods/%v/access_token?&client_id=aokchnui2n8q38g0vezl9hq6htzy4c", vodID)

	fmt.Println("Contacting Twitch Server")

	sig, token, err := accessTokenAPI(tokenAPILink)
	if err != nil {
		fmt.Println("Couldn't access twitch token api")
		os.Exit(1)
	}

	usherAPILink := fmt.Sprintf("http://usher.twitch.tv/vod/%v?nauthsig=%v&nauth=%v&allow_source=true", vodID, sig, token)

	edgecastBaseURL, m3u8Link, err := accessUsherAPI(usherAPILink)
	if err != nil {
		fmt.Println("Count't access usher api")
		os.Exit(1)
	}

	fmt.Println("Getting Video info")

	m3u8List, err := getM3U8List(m3u8Link)
	if err != nil {
		fmt.Println("Couldn't download m3u8 list")
		os.Exit(1)
	}

	targetduration, _ := strconv.Atoi(m3u8List[strings.Index(m3u8List, targetdurationStart)+len(targetdurationStart) : strings.Index(m3u8List, targetdurationEnd)])
	chunkNum := numberOfChunks(vodSH, vodSM, vodSS, vodEH, vodEM, vodES, targetduration)
	startChunk := startingChunk(vodSH, vodSM, vodSS, targetduration)

	var wg sync.WaitGroup
	wg.Add(chunkNum)

	fmt.Println("Starting Download")

	for i := startChunk; i < (startChunk + chunkNum); i++ {

		s := strconv.Itoa(i)
		go downloadChunk(edgecastBaseURL, s, vodIDString, &wg)
	}
	wg.Wait()

	fmt.Println("Combining parts")

	ffmpegCombine(chunkNum, startChunk, vodIDString)

	fmt.Println("Deleting chunks")

	deleteChunks(chunkNum, startChunk, vodIDString)

	fmt.Println("All done!")
}
