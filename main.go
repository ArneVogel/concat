package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/abiosoft/semaphore"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//new style of edgecast links: https://vod089-ttvnw.akamaized.net/1059582120fbff1a392a_reinierboortman_26420932624_719978480/chunked/highlight-180380104.m3u8
//old style of edgecast links: https://vod164-ttvnw.akamaized.net/7a16586e4b7ef40300ba_zizaran_27258736688_772341213/chunked/index-dvr.m3u8

const edgecastLinkBegin string = "https://"
const edgecastLinkBaseEndOld string = "index"
const edgecastLinkBaseEnd string = "highlight"
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
const versionNumber string = "v0.2.5"

var ffmpegCMD string = `ffmpeg`

var debug bool
var twitch_client_id string = "aokchnui2n8q38g0vezl9hq6htzy4c"

var sem *semaphore.Semaphore

/*
	Returns the signature and token from a tokenAPILink
	signature and token are needed for accessing the usher api
*/
func accessTokenAPI(tokenAPILink string) (string, string, error) {
	printDebugf("\ntokenAPILink: %s\n", tokenAPILink)

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

	printDebugf("\nUsher API response:\n%s\n", respString)

	var re = regexp.MustCompile(qualityStart + "([^\"]+)" + qualityEnd + "\n([^\n]+)\n")
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
func calcChunkCount(sh int, sm int, ss int, eh int, em int, es int, target int) int {
	start_seconds := toSeconds(sh, sm, ss)
	end_seconds := toSeconds(eh, em, es)

	return ((end_seconds - start_seconds) / target) + 1
}

func startingChunk(sh int, sm int, ss int, target int) int {
	start_seconds := toSeconds(sh, sm, ss)
	return (start_seconds / target)
}

func toSeconds(sh int, sm int, ss int) int {
	return sh*3600 + sm*60 + ss
}

func downloadChunk(newpath string, edgecastBaseURL string, chunkCount string, chunkName string, vodID string, wg *sync.WaitGroup) {
	defer wg.Done()

	sem.Acquire()

	chunkUrl := edgecastBaseURL + chunkName

	downloadPath := newpath + "/" + vodID + "_" + chunkCount + chunkFileExtension

	if _, err := os.Stat(downloadPath); !os.IsNotExist(err) {
		if debug {
			fmt.Printf("Skipping %s thats already downloaded\n", chunkUrl)
		} else {
			fmt.Print("+")
		}
		sem.Release()
		return
	}

	if debug {
		fmt.Printf("Downloading: %s\n", chunkUrl)
	} else {
		fmt.Print(".")
	}

	httpClient := http.Client{
		Timeout: 30 * time.Second,
	}

	var body []byte

	maxRetryCount := 3
	for retryCount := 0; retryCount < maxRetryCount; retryCount++ {
		if retryCount > 0 {
			printDebugf("%d. retry: chunk '%s'\n", retryCount, chunkName)
		}

		body = nil

		resp, err := httpClient.Get(chunkUrl)

		if err != nil {
			printFatal(err, "Could not download chunk", chunkName)
		}

		if resp.StatusCode != 200 {
			body, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			printDebugf("StatusCode: %d; %s; Could not download chunk '%s'", resp.StatusCode, string(body), chunkUrl)
			return
		}

		body, err = ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {

			if retryCount == maxRetryCount-1 {
				printFatal(err, "Could not download chunk", chunkUrl, "after", maxRetryCount, "tries.")
			} else {
				printDebug("Could not download chunk", chunkUrl)
				printDebug(err)
			}

		} else {
			break
		}

	}

	_ = ioutil.WriteFile(downloadPath, body, 0644)

	sem.Release()
}

func createConcatFile(newpath string, chunkNum int, startChunk int, vodID string) (*os.File, error) {
	tempFile, err := ioutil.TempFile(newpath, "twitchVod_"+vodID+"_")
	if err != nil {
		return nil, err
	}
	defer tempFile.Close()
	concat := ``
	for i := startChunk; i < (startChunk + chunkNum); i++ {
		s := strconv.Itoa(i)
		filePath, _ := filepath.Abs(newpath + "/" + vodID + "_" + s + chunkFileExtension)
		concat += "file '" + filePath + "'\n"
	}

	if _, err := tempFile.WriteString(concat); err != nil {
		return nil, err
	}
	return tempFile, nil
}

func ffmpegCombine(newpath string, chunkNum int, startChunk int, vodID string, vodSavePath string) {
	tempFile, err := createConcatFile(newpath, chunkNum, startChunk, vodID)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.Remove(tempFile.Name())
	args := []string{"-f", "concat", "-safe", "0", "-i", tempFile.Name(), "-c", "copy", "-bsf:a", "aac_adtstoasc", "-fflags", "+genpts", vodSavePath}

	if debug {
		fmt.Printf("Running ffmpeg: %s %s\n", ffmpegCMD, args)
	}

	cmd := exec.Command(ffmpegCMD, args...)
	var errbuf bytes.Buffer
	cmd.Stderr = &errbuf
	err = cmd.Run()
	if err != nil {
		fmt.Println(errbuf.String())
		fmt.Println("ffmpeg error")
	}
}

func deleteChunks(newpath string, chunkCount int, startChunk int, vodID string) {
	var del string
	for i := startChunk; i < (startChunk + chunkCount); i++ {
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

	tokenAPILink := fmt.Sprintf("https://api.twitch.tv/api/vods/%v/access_token?&client_id="+twitch_client_id, vodID)

	fmt.Println("Contacting Twitch Server")

	sig, token, err := accessTokenAPI(tokenAPILink)
	if err != nil {
		printFatal(err, "Could not access twitch token api")
	}

	usherAPILink := fmt.Sprintf("http://usher.twitch.tv/vod/%v?nauthsig=%v&nauth=%v&allow_source=true", vodID, sig, token)

	resp, err := http.Get(usherAPILink)
	if err != nil {
		printFatal(err, "Could not download qualitiy options")
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		printFatal(err, "Could not read qualitiy options")
	}

	respString := string(body)

	qualityCount := strings.Count(respString, resolutionStart)
	for i := 0; i < qualityCount; i++ {
		rs := strings.Index(respString, resolutionStart) + len(resolutionStart)
		re := strings.Index(respString[rs:], resolutionEnd) + rs
		qs := strings.Index(respString, qualityStart) + len(qualityStart)
		qe := strings.Index(respString[qs:], qualityEnd) + qs

		fmt.Printf("resolution: %s, download with -quality=\"%s\"\n", respString[rs:re], respString[qs:qe])

		respString = respString[qe:]
	}
}

func wrongInputNotification() {
	fmt.Println("Call the program with -help for information on how to use it :^)")
}

func downloadPartVOD(vodIDString string, start string, end string, quality string, downloadPath string) {
	var vodID, vodSH, vodSM, vodSS, vodEH, vodEM, vodES int

	vodID, _ = strconv.Atoi(vodIDString)

	if end != "full" {
		startArray := strings.Split(start, " ")
		endArray := strings.Split(end, " ")

		vodSH, _ = strconv.Atoi(startArray[0]) //start Hour
		vodSM, _ = strconv.Atoi(startArray[1]) //start minute
		vodSS, _ = strconv.Atoi(startArray[2]) //start second
		vodEH, _ = strconv.Atoi(endArray[0])   //end hour
		vodEM, _ = strconv.Atoi(endArray[1])   //end minute
		vodES, _ = strconv.Atoi(endArray[2])   //end second

		if toSeconds(vodSH, vodSM, vodSS) > toSeconds(vodEH, vodEM, vodES) {
			wrongInputNotification()
		}
	}

	vodSavePath := filepath.Join(downloadPath, vodIDString+".mp4")

	_, err := os.Stat(vodSavePath)

	if err == nil || !os.IsNotExist(err) {
		printFatalf(err, "Destination file %s already exists!\n", vodSavePath)
	}

	tokenAPILink := fmt.Sprintf("https://api.twitch.tv/api/vods/%v/access_token?&client_id="+twitch_client_id, vodID)

	fmt.Println("Contacting Twitch Server")

	sig, token, err := accessTokenAPI(tokenAPILink)
	if err != nil {
		printFatal(err, "Could not access twitch token api")
	}

	printDebugf("\nSig: %s, Token: %s\n", sig, token)

	usherAPILink := fmt.Sprintf("http://usher.twitch.tv/vod/%v?nauthsig=%v&nauth=%v&allow_source=true", vodID, sig, token)

	printDebugf("\nusherAPILink: %s\n", usherAPILink)

	edgecastURLmap, err := accessUsherAPI(usherAPILink)
	if err != nil {
		printFatal(err, "Count't access usher api")
	}

	printDebug(edgecastURLmap)

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
			for key := range edgecastURLmap {
				key_tmp = strings.Split(key, "p")

				resolution_tmp, _ = strconv.Atoi(key_tmp[0])

				if len(key_tmp) > 1 {
					fps_tmp, _ = strconv.Atoi(key_tmp[1])
				} else {
					fps_tmp = 0
				}

				if resolution_tmp > resolution_max || resolution_tmp == resolution_max && fps_tmp > fps_max {
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
	if strings.Contains(edgecastBaseURL, edgecastLinkBaseEndOld) {
		edgecastBaseURL = edgecastBaseURL[0:strings.Index(edgecastBaseURL, edgecastLinkBaseEndOld)]
	} else {
		edgecastBaseURL = edgecastBaseURL[0:strings.Index(edgecastBaseURL, edgecastLinkBaseEnd)]
	}

	printDebugf("\nedgecastBaseURL: %s\nm3u8Link: %s\n", edgecastBaseURL, m3u8Link)

	fmt.Println("Getting Video info")

	m3u8List, err := getM3U8List(m3u8Link)
	if err != nil {
		printFatal(err, "Couldn't download m3u8 list")
	}

	printDebugf("\nm3u8List:\n%s\n", m3u8List)

	fileUris := readFileUris(m3u8List)

	printDebugf("\nItems list: %v\n", fileUris)

	var chunkCount, startChunk int

	clipDuration := 0

	if end != "full" {
		fileDurations, err := readFileDurations(m3u8List)

		if err != nil || len(fileDurations) != len(fileUris) {
			printDebug("Could not determine real file durations. Using targetDuration as fallback.")
			targetduration, _ := strconv.Atoi(m3u8List[strings.Index(m3u8List, targetdurationStart)+len(targetdurationStart) : strings.Index(m3u8List, targetdurationEnd)])
			chunkCount = calcChunkCount(vodSH, vodSM, vodSS, vodEH, vodEM, vodES, targetduration)
			startChunk = startingChunk(vodSH, vodSM, vodSS, targetduration)
		} else {

			startSeconds := toSeconds(vodSH, vodSM, vodSS)
			clipDuration = toSeconds(vodEH, vodEM, vodES) - startSeconds

			startChunk, chunkCount, _ = calcStartChunkAndChunkCount(fileDurations, startSeconds, clipDuration)
		}

	} else {
		fmt.Println("Downloading full vod")

		chunkCount = len(fileUris)
		startChunk = 0
	}

	printDebugf("\nchunkCount: %v\nstartChunk: %v\n", chunkCount, startChunk)

	var wg sync.WaitGroup
	wg.Add(chunkCount)

	newpath := filepath.Join(downloadPath, "_"+vodIDString)

	err = os.MkdirAll(newpath, os.ModePerm)
	if err != nil {
		printFatal(err, "Could not create directory")
	}
	fmt.Printf("Created temp dir: %s\n", newpath)

	fmt.Println("Starting Download")

	for i := startChunk; i < (startChunk + chunkCount); i++ {

		s := strconv.Itoa(i)
		n := fileUris[i]
		go downloadChunk(newpath, edgecastBaseURL, s, n, vodIDString, &wg)
	}
	wg.Wait()

	fmt.Println("\nCombining parts")

	ffmpegCombine(newpath, chunkCount, startChunk, vodIDString, vodSavePath)

	fmt.Println("Deleting chunks")

	deleteChunks(newpath, chunkCount, startChunk, vodIDString)

	fmt.Println("Deleting temp dir")

	os.Remove(newpath)

	fmt.Println("All done!")
}

func calcStartChunkAndChunkCount(chunkDurations []float64, startSeconds int, clipDuration int) (int, int, float64) {
	startChunk := 0
	chunkCount := 0
	startSecondsRemainder := float64(0)

	cumulatedDuration := 0.0
	for chunk, chunkDuration := range chunkDurations {
		cumulatedDuration += chunkDuration

		if cumulatedDuration > float64(startSeconds) {
			startChunk = chunk
			startSecondsRemainder = float64(startSeconds) - (cumulatedDuration - chunkDuration)
			break
		}
	}

	cumulatedDuration = 0.0
	minChunkedClipDuration := float64(clipDuration) + startSecondsRemainder
	for chunk := startChunk; chunk < len(chunkDurations); chunk++ {
		cumulatedDuration += chunkDurations[chunk]

		if cumulatedDuration > minChunkedClipDuration {
			chunkCount = chunk - startChunk + 1
			break
		}
	}

	if chunkCount == 0 {
		chunkCount = len(chunkDurations) - startChunk
	}

	return startChunk, chunkCount, startSecondsRemainder
}

func readFileUris(m3u8List string) []string {
	var fileRegex = regexp.MustCompile("(?m:^[^#\\n]+)")
	matches := fileRegex.FindAllStringSubmatch(m3u8List, -1)
	var ret []string
	for _, match := range matches {
		ret = append(ret, match[0])
	}
	return ret
}

func readFileDurations(m3u8List string) ([]float64, error) {
	var fileRegex = regexp.MustCompile("(?m:^#EXTINF:(\\d+(\\.\\d+)?))")
	matches := fileRegex.FindAllStringSubmatch(m3u8List, -1)

	var ret []float64

	for _, match := range matches {

		fileLength, err := strconv.ParseFloat(match[1], 64)

		if err != nil {
			printDebug(err)
			return nil, err
		}

		ret = append(ret, fileLength)
	}

	return ret, nil
}

func rightVersion() bool {
	resp, err := http.Get(currentReleaseLink)
	if err != nil {
		printFatal(err, "Could not access github while checking for most recent release.")
	}

	body, _ := ioutil.ReadAll(resp.Body)

	respString := string(body)

	cs := strings.Index(respString, currentReleaseStart) + len(currentReleaseStart)
	ce := cs + len(versionNumber)
	return respString[cs:ce] == versionNumber
}

//
func ffmpegIsInstalled() bool {
	out, _ := exec.Command(ffmpegCMD).Output()
	return out != nil
}

func init() {
	if runtime.GOOS == "windows" {
		ffmpegCMD = `ffmpeg.exe`
	}

	if !ffmpegIsInstalled() {
		fmt.Println("Could not find ffmpeg, make sure to have ffmpeg avaliable on your system.")
		os.Exit(1)
	}
}

func main() {

	qualityInfo := flag.Bool("qualityinfo", false, "if you want to see the avaliable quality options")

	standardStartAndEnd := "HH MM SS"
	standardVOD := "123456789"
	vodID := flag.String("vod", standardVOD, "the vod id https://www.twitch.tv/videos/123456789")
	start := flag.String("start", standardStartAndEnd, "For example: 0 0 0 for starting at the beginning of the vod")
	end := flag.String("end", standardStartAndEnd, "For example: 1 20 0 for ending the vod at 1 hour and 20 minutes")
	quality := flag.String("quality", sourceQuality, "chunked for source quality is automatically used if -quality isn't set")
	debugFlag := flag.Bool("debug", false, "debug output")
	semaphoreLimit := flag.Int("max-concurrent-downloads", 5, "change maximum number of concurrent downloads")
	downloadPath := flag.String("download-path", ".", "path where the file will be saved")

	flag.Parse()

	debug = *debugFlag
	sem = semaphore.New(*semaphoreLimit)

	if !rightVersion() {
		fmt.Printf("\nYou are using an old version of concat. Check out %s for the most recent version.\n\n", currentReleaseLink)
	}

	if *vodID == standardVOD {
		wrongInputNotification()
		os.Exit(1)
	}

	if *qualityInfo {
		printQualityOptions(*vodID)
		os.Exit(0)
	}

	if *start != standardStartAndEnd && *end != standardStartAndEnd {
		downloadPartVOD(*vodID, *start, *end, *quality, *downloadPath)
	} else {
		downloadPartVOD(*vodID, "0", "full", *quality, *downloadPath)
	}
}
