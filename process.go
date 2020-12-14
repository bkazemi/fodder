// video transformation
package main

import (
  "fmt"
  "os"
  "unsafe"
//  "log"
  "bytes"
  "image"
  "image/jpeg"
  "regexp"
  "reflect"
  "errors"

  "github.com/giorgisio/goav/swscale"
  "github.com/giorgisio/goav/avcodec"
  "github.com/giorgisio/goav/avformat"
  "github.com/giorgisio/goav/avutil"
)

import "C"

type Game int

// games allowed
const (
  G_OTHER    Game = 0
  G_VALORANT Game = 1
  G_CSGO     Game = 2
)

type KillLogVid struct {
  Vid      *VideoInfo     // video to be cropped
  Game      Game          // game being cropped
  Match    *regexp.Regexp // regexp of username
  FrameChan chan []byte   // channel used to send image to OCR engine
}

// constructor used to make channel allocation easier
func (klv *KillLogVid) Init() {
  klv.FrameChan = make(chan []byte, 50) // TODO add knob
}

// methods that crop streams to kill log portion only
func (KillLogVid) CropValorant(klv *KillLogVid) error {
  pullText := &PullText{}
  pullText.Init(klv)
  if err := video2Image(klv, pullText); err != nil {
    return err
  }
  if err := pullText.Finish(klv); err != nil {
    return err
  }
  fmt.Println("finished scanning valorant stream.") // TODO: add title

  return nil
}

// returns a Rect() of portion of video we want to scan for text
func gameSubImage(klv *KillLogVid) (image.Rectangle, error) {
  nilRect := image.Rect(0,0,0,0) // FIXME: find out why i cant use nil
  switch klv.Game {
  case G_VALORANT:
    if klv.Vid.Res == "1080p" || klv.Vid.Res == "1080p60" { // XXX: add support for other resolutions
      return image.Rect(1455, 83, 1899, 339), nil
    }
    fmt.Printf("%s\n", klv.Vid.Res)
    return nilRect, errors.New("unsupported resolution.") // NOTE: part of above XXX
  case G_CSGO:
    return nilRect, errors.New("BUG: gameSubImage(): CSGO is not currently supported. " +
                               "This should be checked before this message.")
  case G_OTHER:
    return nilRect, errors.New("BUG: gameSubImage(): invalid game used. This should be " +
                               "checked before this message.")
  default:
    panic("[process] gameSubImage(): invalid game")
  }
}

// saveFrame creates an in-memory Image of a single frame
func saveFrame(klv *KillLogVid, frame *avutil.Frame, width int, height int) ([]byte, error) {
  img := image.NewRGBA(image.Rect(0, 0, width, height))

  // write pixel data
  for pixIdx, y := 0, 0; y < height; y++ {
    data0 := avutil.Data(frame)[0]
    startPos := uintptr(unsafe.Pointer(data0)) + uintptr(y)*uintptr(avutil.Linesize(frame)[0])
    for i := 0; i < width*4; i++ { // width*4 == r,g,b,a
      element := *(*uint8)(unsafe.Pointer(startPos + uintptr(i)))
      img.Pix[pixIdx] = element
      pixIdx++
    }
  }
  subImgRect, err := gameSubImage(klv)
  if err != nil {
    return nil, err
  }
  croppedFrame := img.SubImage(subImgRect)
  var jpgBuf bytes.Buffer
  jpeg.Encode(&jpgBuf, croppedFrame, nil)
  return jpgBuf.Bytes(), nil
}

// convert video to an image with ffmpeg and send to PullText()
// code modified from goav tutorial
func video2Image(klv *KillLogVid, pt *PullText) (error) {
  // open video file
  v := klv.Vid
  pFormatContext := avformat.AvformatAllocContext()
  if avformat.AvformatOpenInput(&pFormatContext, v.File, nil, nil) != 0 {
    return fmt.Errorf("unable to open file %s\n", v.File)
  }

  // retrieve stream information
  if pFormatContext.AvformatFindStreamInfo(nil) < 0 {
    return errors.New("couldn't find stream information")
  }

  // dump information about file onto standard error
  // XXX: for debug
  //pFormatContext.AvDumpFormat(0, v.File, 0)

  // find the first video stream
  for i := 0; i < int(pFormatContext.NbStreams()); i++ {
    switch pFormatContext.Streams()[i].CodecParameters().AvCodecGetType() {
    case avformat.AVMEDIA_TYPE_VIDEO:
      stream := pFormatContext.Streams()[i]
      // get a pointer to the codec context for the video stream
      pCodecCtxOrig := stream.Codec()
      // find the decoder for the video stream
      pCodec := avcodec.AvcodecFindDecoder(avcodec.CodecId(pCodecCtxOrig.GetCodecId()))
      if pCodec == nil {
        // XXX: add granularity
        return errors.New("unsupported codec")
      }
      // copy context
      pCodecCtx := pCodec.AvcodecAllocContext3()
      if pCodecCtx.AvcodecCopyContext((*avcodec.Context)(unsafe.Pointer(pCodecCtxOrig))) != 0 {
        return errors.New("couldn't copy codec context")
      }

      // open codec
      if pCodecCtx.AvcodecOpen2(pCodec, nil) < 0 {
        return errors.New("couldn't open codec")
      }

      // allocate video frame
      pFrame := avutil.AvFrameAlloc()

      // allocate an AVFrame structure
      pFrameRGBA := avutil.AvFrameAlloc()
      if pFrameRGBA == nil {
        return errors.New("unable to allocate an RGBA Frame")
      }

      // determine required buffer size and allocate buffer
      numBytes := uintptr(avcodec.AvpictureGetSize(avcodec.AV_PIX_FMT_RGBA, pCodecCtx.Width(),
                          pCodecCtx.Height()))
      buffer := avutil.AvMalloc(numBytes)

      // assign appropriate parts of buffer to image planes in pFrameRGBA
      // note that pFrameRGBA is an AVFrame, but AVFrame is a superset
      // of AVPicture
      avp := (*avcodec.Picture)(unsafe.Pointer(pFrameRGBA))
      avp.AvpictureFill((*uint8)(buffer), avcodec.AV_PIX_FMT_RGBA, pCodecCtx.Width(), pCodecCtx.Height())

      // initialize SWS context for software scaling
      swsCtx := swscale.SwsGetcontext(
        pCodecCtx.Width(),
        pCodecCtx.Height(),
        (swscale.PixelFormat)(pCodecCtx.PixFmt()),
        pCodecCtx.Width(),
        pCodecCtx.Height(),
        avcodec.AV_PIX_FMT_RGBA,
        avcodec.SWS_BILINEAR,
        nil,
        nil,
        nil,
      )

      fpsReflect := reflect.ValueOf(stream).Elem().FieldByName("avg_frame_rate")//("time_base")
      fpsNum := *(*int32)(unsafe.Pointer(fpsReflect.Field(0).UnsafeAddr()))
      fpsDem := *(*int32)(unsafe.Pointer(fpsReflect.Field(1).UnsafeAddr()))
      fps := uint64(0)
      if fpsNum == 0 || fpsDem == 0 {
        fmt.Println("couldn't get video FPS, fallback to reading every frame")
        fps, fpsNum, fpsDem = 0, 0, 0
      } else {
        if fpsNum > fpsDem {
          fpsDem, fpsNum = fpsNum, fpsDem // swap variables in case of endianness problems
        }
        fps = uint64(fpsDem / fpsNum)
        fps = fps / 2 // every .5 sec XXX remove me!
        fmt.Printf("video fps: %d scanning x2\n", fps)
      }

      // read frames
      frameNum   := uint64(0)
      packet := avcodec.AvPacketAlloc()
      for pFormatContext.AvReadFrame(packet) >= 0 {
        // is this a packet from the video stream?
        if packet.StreamIndex() == i {
          // decode video frame
          response := pCodecCtx.AvcodecSendPacket(packet)
          if response < 0 {
            fmt.Printf("Error while sending a packet to the decoder: %s\n", avutil.ErrorFromCode(response))
          }
          for response >= 0 {
            response = pCodecCtx.AvcodecReceiveFrame((*avcodec.Frame)(unsafe.Pointer(pFrame)))
            if response == avutil.AvErrorEAGAIN || response == avutil.AvErrorEOF ||
               response == -11 /* FIXME tmp */ {
              break
            } else if response < 0 {
              fmt.Printf("Error while receiving a frame from the decoder: %s\n", avutil.ErrorFromCode(response))
              //fmt.Println(response)
              return errors.New("video2Image(): decoder err")
            }
            // get 1 fps
            // TODO replace w/ libavfilter
            if frameNum % fps != 0 {
              packet.AvFreePacket()
              frameNum++
              continue
            }

            // convert to RGBA
            swscale.SwsScale2(swsCtx, avutil.Data(pFrame),
                              avutil.Linesize(pFrame), 0, pCodecCtx.Height(),
                              avutil.Data(pFrameRGBA), avutil.Linesize(pFrameRGBA))

            img, err := saveFrame(klv, pFrameRGBA, pCodecCtx.Width(), pCodecCtx.Height())
            if err != nil {
              return err
            }

            if pt.err != nil {
              return nil // problem with PullText(), don't add a superfluous error
            }

            klv.FrameChan <-img

            frameNum++
          }
        }

        // free the packet that was allocated by av_read_frame
        packet.AvFreePacket()
      }

      // free the RGBA image
      avutil.AvFree(buffer)
      avutil.AvFrameFree(pFrameRGBA)

      // free the YUV frame
      avutil.AvFrameFree(pFrame)

      // close the codecs
      pCodecCtx.AvcodecClose()
      (*avcodec.Context)(unsafe.Pointer(pCodecCtxOrig)).AvcodecClose()

      // close the video file
      pFormatContext.AvformatCloseInput()

      // stop after saving frames of first video straem
      break

    default:
      fmt.Println("couldn't find a video stream")
      os.Exit(1)
    }
  }

  return nil
}
