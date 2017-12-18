# concat
## Download
Go to [releases](https://github.com/ArneVogel/concat/releases) for the current build. Download concat.exe if you are on windows and concat if want to use it on ubuntu.

## Prerequisite
You need ffmpeg for this tool to work. On windows you can get it [here](https://www.ffmpeg.org/download.html). 
On Ubuntu "sudo apt-get install ffmpeg" will work.

## Usage
Calling options:
+ -vod `-vod="123456789"` specify what vod you want to download or want quality informations on. Call with the number you find in the url of the vod eg (https://www.twitch.tv/videos/123456789 => __123456789__)
+ -start `-start="0 0 0"`
+ -end `-start="1 20 30"`
+ -quality `-start="720p60"` if you don't set the quality concat will try to download the vod in the highest available quality, see -qualityinfo for all available quality options for each vod
+ -qualityinfo `-qualityinfo`

VODID 
Start and End are given in the format `HH MM SS`, HH is hours, MM is minuts, SS is seconds 
### Windows
**Make sure that ffmpeg is in the same directory as concat.exe**.
Call `concat.exe VODID HH MM SS HH MM SS`
### Ubuntu
Call `./concat VODID HH MM SS HH MM SS`

## More info
[Blog post](https://www.arnevogel.com/standalone-concat-version/) about the tool.

Send me feedback contact@concat.org