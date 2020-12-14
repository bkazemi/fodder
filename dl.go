// video download related routines
package main

import (
  "io"
  "io/ioutil"
  "os"
  "fmt"
  "strconv"
  "time"
  "regexp"
  "errors"
  "log"

  "github.com/kkdai/youtube/v2"
)

type VideoInfo struct {
  File string //FIXME*os.File
  Title string
  Width int
  Height int
  Res string
}

func getYTVID (url string) (string, error) {
  r, _ := regexp.Compile("^(http(s)?://)?(www\\.)?(youtube\\.com/+watch\\?v=|youtu\\.be/+)")

  domainStr := r.FindStringIndex(url)
  if len(domainStr) != 2 {
    return "", errors.New("bad URL")
  }

  return url[domainStr[1]:], nil
}

func DownloadVideo(url string, path string) (*VideoInfo, error) {
  // for now, only support YouTube
  if path != "" {
    return fetchYoutubeVideoInfo(url, path)
  } else {
    return downloadYoutubeVideo(url, path)
  }
}

// FIXME: rough impl
func fetchYoutubeVideoInfo(url string, path string) (*VideoInfo, error) {
  var vidInfo VideoInfo
  vid, err := getYTVID(url)
  if err != nil {
    return nil, err
  }

  client := youtube.Client{}
  video, err := client.GetVideo(vid)
  if err != nil {
    return nil, err
  }

  sIdx := -1
  for i, f := range video.Formats {
    switch f.QualityLabel {
    case "1080p60", "1080p", "720p60", "720p", "480p", "360p":
      if sIdx == -1 {
        // first match
        vidInfo.Width  = f.Width
        vidInfo.Height = f.Height
        vidInfo.Res    = f.QualityLabel
        sIdx = i
      } else if  f.Height > vidInfo.Height ||
                (f.Height == vidInfo.Height && string(f.Quality[len(f.Quality)-2]) == "60") /* 60 fps */ {
        // prefer better quality
        vidInfo.Width  = f.Width
        vidInfo.Height = f.Height
        vidInfo.Res    = f.QualityLabel
        sIdx = i
      }
    }
  }
  if sIdx == -1 {
    return nil, errors.New("couldn't find a video stream with appropriate resolution")
  }

  vidInfo.File  = path
  vidInfo.Title = video.Title

  return &vidInfo, nil
}

func downloadYoutubeVideo(url string, path string) (*VideoInfo, error) {
  var vidInfo VideoInfo

  vid, err := getYTVID(url)
  if err != nil {
    return nil, err
  }

  client := youtube.Client{}
  video, err := client.GetVideo(vid)
  if err != nil {
    return nil, err
  }

  sIdx := -1
  for i, f := range video.Formats {
    switch f.QualityLabel {
    case "1080p60", "1080p", "720p60", "720p", "480p", "360p":
      if sIdx == -1 {
        // first match
        vidInfo.Width  = f.Width
        vidInfo.Height = f.Height
        vidInfo.Res    = f.QualityLabel
        sIdx = i
      } else if  f.Height > vidInfo.Height ||
                (f.Height == vidInfo.Height && string(f.Quality[len(f.Quality)-2]) == "60") /* 60 fps */ {
        // prefer better quality
        vidInfo.Width  = f.Width
        vidInfo.Height = f.Height
        vidInfo.Res    = f.QualityLabel
        sIdx = i
      }
    }
  }
  if sIdx == -1 {
    return nil, errors.New("couldn't find a video stream with appropriate resolution")
  }

  log.Print("requesting stream...")
  resp, err := client.GetStream(video, &video.Formats[sIdx])
  if err != nil {
    return nil, err
  }
  defer resp.Body.Close()
  streamSz, _ := strconv.Atoi(video.Formats[sIdx].ContentLength)

  file, err := ioutil.TempFile("", vid + ".mp4")
  if err != nil {
     return nil, err
  }
  defer file.Close() //FIXME

  log.Print("saving a temporary copy to disk...")
  videoDone := make(chan bool)
  go progress(video.Title, vidInfo.Res, file.Name(), uint(streamSz), videoDone)
  _, err = io.Copy(file, resp.Body)
  if err != nil {
    return nil, err
  }
  videoDone <- true
  <-videoDone // ensure we finish progress()'s output before exiting

  vidInfo.File  = file.Name() //FIXME
  vidInfo.Title = video.Title

  return &vidInfo, nil
}

//type progress_bufsz(youtube.Video)

// NOTE: path is expected to be valid
func progress(title string, res string, path string, total_sz uint, done chan bool) {
  tick := time.NewTicker(700 * time.Millisecond) // tick every .7 seconds for fluid progress
  total_sz = uint(total_sz / 1e6) // to MB
  if title != "" {
    fmt.Printf("[title]: `%s` @ %s\n", title, res)
  }
  for {
    select {
    case <-tick.C:
      f_stat, err := os.Stat(path)
      if err != nil {
        log.Print("progress(): couldn't stat() the file.")
        return
      }
      f_sz := f_stat.Size() / 1e6
      pcnt := (float64(f_sz) / float64(total_sz)) * 100
      //f_len := uint(len(string(f_sz)))
      // carriage return to erase current output
      fmt.Printf("\r")
      // print new file size
      if total_sz != 0 {
        fmt.Printf("downloaded %d/%dMB (%d%%)", uint(f_sz), uint(total_sz), uint(pcnt))
      } else {
        fmt.Printf("downloaded %dMB", uint(f_sz))
      }
    case <-done:
      tick.Stop()
      fmt.Printf(" done!\n")
      done <- true
      return
    }
  }
}
