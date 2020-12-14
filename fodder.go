package main

import (
  "fmt"
  "flag"
  "regexp"
)

const usageString = `usage: fodder [option] <url>
Scan a video stream for your username.
`

type options struct {
  userNameRegexp *regexp.Regexp
  url             string
  videoFilePath   string
  verbose         bool
}

func main() {
  var ( // arguments that need parsing
    userNameExpr  string
    videoFilePath string
    verbose       bool
  )
  flag.Usage = func() {
    fmt.Println(usageString)
    flag.PrintDefaults()
  }
  flag.StringVar(&userNameExpr, "u", "", "regexp of username to scan for")
  flag.StringVar(&videoFilePath, "f", "", "stream video file")
  flag.BoolVar(&verbose, "v", false, "show verbose messages")
  flag.Parse()

  if (len(flag.Args()) == 0 || flag.Arg(0) == "") && videoFilePath == "" {
    fmt.Println("error: no URL provided")
    flag.Usage()
    return
  }

  if (userNameExpr == "") {
    fmt.Println("error: -u required")
    flag.Usage()
  }
  userNameRegexp, err := regexp.Compile(userNameExpr)
  if err != nil {
    fmt.Println("error: bad regexp string for `-u`")
    flag.Usage()
  }

  opts := &options{
    url:            flag.Arg(0),
    userNameRegexp: userNameRegexp,
    videoFilePath:  videoFilePath,
    verbose:        verbose,
  }

  if err := run(opts); err != nil {
    fmt.Println(err)
    return
  }
}

func run(opts *options) error {
  vid, err := DownloadVideo(opts.url, opts.videoFilePath)
  if err != nil {
    return err
  }
  fmt.Println("download successful. processing video...")
  killLog := &KillLogVid{
    Vid:   vid,
    Match: opts.userNameRegexp,
    Game:  G_VALORANT,
  }
  killLog.Init()
  if err := killLog.CropValorant(killLog); err != nil {
    return fmt.Errorf("CropValorant() err: %s", err)
  }
  fmt.Println("all streams processed successfully.")

  return nil
}
