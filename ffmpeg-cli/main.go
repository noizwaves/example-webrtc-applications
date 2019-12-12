package main

import (
	"fmt"
	"math/rand"
	"os/exec"
	"time"

	"github.com/pion/example-webrtc-applications/internal/signal"
	"github.com/pion/webrtc/v2"
	"github.com/pion/webrtc/v2/pkg/media"
)

func main() {
	// Wait for the offer to be pasted
	offer := webrtc.SessionDescription{}
	signal.Decode(signal.MustReadStdin(), &offer)

	// We make our own mediaEngine so we can place the sender's codecs in it.  This because we must use the
	// dynamic media type from the sender in our answer. This is not required if we are the offerer
	mediaEngine := webrtc.MediaEngine{}
	err := mediaEngine.PopulateFromSDP(offer)
	if err != nil {
		panic(err)
	}

	// Search for VP8 Payload type. If the offer doesn't support VP8 exit since
	// since they won't be able to decode anything we send them
	var payloadType uint8
	for _, videoCodec := range mediaEngine.GetCodecsByKind(webrtc.RTPCodecTypeVideo) {
		fmt.Printf("Found codec type: %s\n", videoCodec.Name)
		if videoCodec.Name == "VP8" {
			payloadType = videoCodec.PayloadType
			fmt.Printf("Clock rate: %d\n", videoCodec.ClockRate)
			break
		}
	}
	if payloadType == 0 {
		panic("Remote peer does not support VP8")
	}

	// Create a new RTCPeerConnection
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	// Create a video track
	videoTrack, err := peerConnection.NewTrack(payloadType, rand.Uint32(), "video", "pion")
	if err != nil {
		panic(err)
	}
	if _, err = peerConnection.AddTrack(videoTrack); err != nil {
		panic(err)
	}

	// Create an opus audio track
	audioTrack, err := peerConnection.NewTrack(webrtc.DefaultPayloadTypeOpus, rand.Uint32(), "audio", "pion")
	if err != nil {
		panic(err)
	}
	if _, err = peerConnection.AddTrack(audioTrack); err != nil {
		panic(err)
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())

		if connectionState.String() == "connected" {
			// start ffmpeg video
			go func() {
				cmd := exec.Command( //nolint
					"ffmpeg",
					"-f", "avfoundation",
					"-re",
					"-i", "1:",
					// h264
					// "-f", "h264",

					// vp8
					/*"-vf", "scale=1440:900",*/
					"-c:v", "libvpx", "-b:v", "1M", "-c:a", "libvorbis", "-f", "webm",
					"pipe:",
				)

				// we will read the video stream from the stdout reader pipe
				out, cmdErr := cmd.StdoutPipe()
				if cmdErr != nil {
					panic(err)
				}

				go func() {
					// start the command
					if err = cmd.Start(); err != nil {
						panic(err)
					}
				}()

				buf := make([]byte, 1024*512)
				for {
					// log the start time to calc samples
					start := time.Now()

					// read stdout pipe data
					n, readErr := out.Read(buf)
					// if n == 0 {
					// 	fmt.Print("Did not read any data")
					// 	continue
					// }
					fmt.Printf("buf.length is %d\n", n)
					if readErr != nil {
						fmt.Printf("Tried to read some data %d\n", n)
						// panic(readErr)
						// fmt.Printf("exited with error %s", cmd.Wait())
						// continue
					}

					// get this time duration
					duration := time.Since(start)

					// vp8 clock rate is 90kHz, calc with this time duration
					samples := uint32(90000 / 1000 * duration.Milliseconds())
					fmt.Printf("samples is %d\n", samples)

					// output to videoTrack
					if err = videoTrack.WriteSample(media.Sample{Data: buf[:n], Samples: samples}); err != nil {
						panic(err)
					}

					// _, writeErr := videoTrack.Write(buf[:n])
					// if writeErr != nil {
					// 	panic(writeErr)
					// }
				}
			}()
		}
	})

	// Set the remote SessionDescription
	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		panic(err)
	}

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Sets the LocalDescription, and starts our UDP listeners
	if err = peerConnection.SetLocalDescription(answer); err != nil {
		panic(err)
	}

	// Output the answer in base64 so we can paste it in browser
	fmt.Println(signal.Encode(answer))

	// Block forever
	select {}
}
