# concat
## Download
Go to [releases](https://github.com/ArneVogel/concat/releases) for the current build. Download concat.exe if you are on windows and concat if want to use it on ubuntu.

## Prerequisite
You need ffmpeg for this tool to work. On windows you can get it [here](https://www.ffmpeg.org/download.html). 
On Ubuntu "sudo apt-get install ffmpeg" will work.

## Usage
VODID is the number you find in the url of the vod eg (https://www.twitch.tv/videos/123456789 => 123456789), the first HH MM SS is the start time and the second HH MM SS is the end time with HH = hours, MM = minutes, SS = seconds.
### Windows
**Make sure that ffmpeg is in the same directory as concat.exe**.
Call `concat.exe VODID HH MM SS HH MM SS`
### Ubuntu
Call `./concat VODID HH MM SS HH MM SS`

## More info
[Blog post](https://www.arnevogel.com/standalone-concat-version/) about the tool.

Send me feedback contact@concat.org