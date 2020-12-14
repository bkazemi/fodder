// OCR routines to pull text from stream
package main

import (
  "fmt"
  "strconv"
  "sync"
  "io/ioutil"

  "github.com/otiai10/gosseract/v2"
)

// total goroutines to allow
const _PT_GOCNT = 10

type PullText struct {
  wg      *sync.WaitGroup
  err      error // avoid context pkg for speed
  errMutex sync.Mutex
}

func (pt *PullText) Init(klv *KillLogVid) {
  var wg sync.WaitGroup

  pt.wg = &wg
  pt.err = nil

  for i := 0; i < _PT_GOCNT; i++ {
    wg.Add(1)
    go pull(pt, klv, i)
  }
}

func (pt *PullText) Finish(klv *KillLogVid) error {
  close(klv.FrameChan)
  pt.wg.Wait()
  fmt.Println()
  if pt.err != nil {
    return pt.err
  }
  if matchcnt == 0 {
    fmt.Printf("no matches found.\n")
  }

  return nil
}

func (pt *PullText) handleError(err error) {
  if pt.err != nil { // only want first error
    return
  }
  pt.errMutex.Lock()
  pt.err = err
  pt.errMutex.Unlock()
}

// worker goroutines for text extraction, main code
var frameimgcnt int = 0  // XXX tmp
var matchcnt    int = 1  // XXX tmp
func pull(pt *PullText, klv *KillLogVid, goNum int) {
  ocrClient := gosseract.NewClient()
  frameChan := klv.FrameChan

  defer ocrClient.Close()
  defer pt.wg.Done()

  for img := range frameChan {
    if pt.err != nil {
      return
    }

    err := ocrClient.SetImageFromBytes(img)
    if err != nil {
      pt.handleError(fmt.Errorf("[ocr] SetImageFromBytes() error: %s\n", err))
      return
    }

    txt, err := ocrClient.Text()
    if err != nil {
      pt.handleError(fmt.Errorf("[ocr] Text() error: %s\n", err))
      return
    }
    if txt != "" {
      if klv.Match.MatchString(txt) {
        if frameimgcnt != 0 {
          fmt.Println()
        }
        fmt.Printf("[pullText][%d]: found match for `%s`\n", goNum, klv.Match.String())
        if err := ioutil.WriteFile("matches/"+klv.Match.String()+"_"+strconv.Itoa(matchcnt)+".jpg", img, 0644); err != nil {
          pt.handleError(fmt.Errorf("[ocr] WriteFile() error: %s\n", err))
          return
        }
        matchcnt++
      }
    }

    fmt.Printf("\rscanned %d frames", frameimgcnt)
    frameimgcnt++
  }
}
