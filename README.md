# concat

## Download

Go to [releases](https://github.com/ArneVogel/concat/releases) for the current build. Download concat.exe if you are on Windows and concat if want to use it on Ubuntu.

## Prerequisite

You need ffmpeg for this tool to work. On Windows you can get it [here](https://www.ffmpeg.org/download.html).
On Ubuntu "sudo apt-get install ffmpeg" will work.

## Usage

You have to call concat from the console.

Calling options:

- -vod `-vod="123456789"` specify what vod you want to download or want quality informations on. Call with the number you find in the url of the vod eg (https://www.twitch.tv/videos/123456789 => **123456789**)
- -start `-start="0 0 0"` (default: from the start)
- -end `-end="1 20 30"` (default: till the end)
- -quality `-quality="720p60"` if you don't set the quality concat will try to download the vod in the highest available quality, see -qualityinfo for all available quality options for each vod
- -qualityinfo `-qualityinfo`
- -max-concurrent-downloads `-max-concurrent-downloads 5` change the number of chunks that concat will attempt to download simultaneously
- -download-path `-download-path="../path/to/dir"` specify where the chunks and end file should be downloaded. By default it is your current working directory
- -filename `-filename="myfile"` name of the final output file (without extension). By default it is the `vodID`
- -audio `-audio` extracts the audio from the video file into a mp3
- -audio-only `-audio-only` same as `-audio` however doesn't keep the video file
- -try-count `-try-count=5` amount of times concat should try fetching chunks. Set to 0 for infinite retries

### MacOS

When downloading the file, if using Safari, the extension will sometimes be switched from no extension to a .dms file, so you have to remove the extension.

Once you get the file without an extension, you have to run `chmod +x ./concat_mac` in terminal to associate the file as a unix executable or else terminal won't allow you to run it.

## Deploy to Heroku version

https://github.com/gyfis/concat-web

## More info

[Blog post](https://www.arnevogel.com/standalone-concat-version/) about the tool.

Send me feedback contact@concat.org
