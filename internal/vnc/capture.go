// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package vnc

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"time"
)

// CaptureScreenshot connects to the VNC server and captures the current screen
// using the raw RFB 3.8 protocol (no external library).
func CaptureScreenshot(ctx context.Context, host string, port int) (*image.RGBA, error) {
	addr := fmt.Sprintf("%s:%d", host, port)

	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to VNC server at %s: %w", addr, err)
	}
	defer conn.Close()

	// Set a deadline based on context or a reasonable timeout
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	} else {
		conn.SetDeadline(time.Now().Add(30 * time.Second))
	}

	// Step 1: Read server protocol version (12 bytes: "RFB 003.008\n")
	serverVersion := make([]byte, 12)
	if _, err := readFull(conn, serverVersion); err != nil {
		return nil, fmt.Errorf("failed to read server protocol version: %w", err)
	}

	// Step 2: Send client protocol version
	clientVersion := []byte("RFB 003.008\n")
	if _, err := conn.Write(clientVersion); err != nil {
		return nil, fmt.Errorf("failed to send client protocol version: %w", err)
	}

	// Step 3: Read security types
	numSecTypes := make([]byte, 1)
	if _, err := readFull(conn, numSecTypes); err != nil {
		return nil, fmt.Errorf("failed to read security types count: %w", err)
	}

	if numSecTypes[0] == 0 {
		// Server sent an error reason
		reasonLen := make([]byte, 4)
		if _, err := readFull(conn, reasonLen); err != nil {
			return nil, fmt.Errorf("failed to read error reason length: %w", err)
		}
		reason := make([]byte, binary.BigEndian.Uint32(reasonLen))
		if _, err := readFull(conn, reason); err != nil {
			return nil, fmt.Errorf("failed to read error reason: %w", err)
		}
		return nil, fmt.Errorf("VNC server refused connection: %s", string(reason))
	}

	secTypes := make([]byte, numSecTypes[0])
	if _, err := readFull(conn, secTypes); err != nil {
		return nil, fmt.Errorf("failed to read security types: %w", err)
	}

	// Step 4: Send security type 1 (None)
	hasNone := false
	for _, t := range secTypes {
		if t == 1 {
			hasNone = true
			break
		}
	}
	if !hasNone {
		return nil, fmt.Errorf("VNC server does not support None authentication (available types: %v)", secTypes)
	}

	if _, err := conn.Write([]byte{1}); err != nil {
		return nil, fmt.Errorf("failed to send security type: %w", err)
	}

	// Step 5: Read SecurityResult (4 bytes, must be 0 for success)
	secResult := make([]byte, 4)
	if _, err := readFull(conn, secResult); err != nil {
		return nil, fmt.Errorf("failed to read security result: %w", err)
	}

	if binary.BigEndian.Uint32(secResult) != 0 {
		// Try to read error reason
		reasonLen := make([]byte, 4)
		if _, err := readFull(conn, reasonLen); err == nil {
			reason := make([]byte, binary.BigEndian.Uint32(reasonLen))
			readFull(conn, reason)
			return nil, fmt.Errorf("VNC authentication failed: %s", string(reason))
		}
		return nil, fmt.Errorf("VNC authentication failed (result: %d)", binary.BigEndian.Uint32(secResult))
	}

	// Step 6: Send ClientInit (shared flag = 1)
	if _, err := conn.Write([]byte{1}); err != nil {
		return nil, fmt.Errorf("failed to send ClientInit: %w", err)
	}

	// Step 7: Read ServerInit
	// width(2) + height(2) + pixel_format(16) + name_length(4) = 24 bytes
	serverInit := make([]byte, 24)
	if _, err := readFull(conn, serverInit); err != nil {
		return nil, fmt.Errorf("failed to read ServerInit: %w", err)
	}

	fbWidth := binary.BigEndian.Uint16(serverInit[0:2])
	fbHeight := binary.BigEndian.Uint16(serverInit[2:4])
	nameLen := binary.BigEndian.Uint32(serverInit[20:24])

	// Read desktop name
	desktopName := make([]byte, nameLen)
	if _, err := readFull(conn, desktopName); err != nil {
		return nil, fmt.Errorf("failed to read desktop name: %w", err)
	}

	// Step 8: Set pixel format to 32bpp BGRA
	// Message type 0 = SetPixelFormat
	setPixelFormat := make([]byte, 20)
	setPixelFormat[0] = 0 // message-type: SetPixelFormat
	// bytes 1-3: padding
	// Pixel format (16 bytes starting at offset 4):
	setPixelFormat[4] = 32                                 // bits-per-pixel
	setPixelFormat[5] = 24                                 // depth
	setPixelFormat[6] = 0                                  // big-endian-flag (little-endian)
	setPixelFormat[7] = 1                                  // true-colour-flag
	binary.BigEndian.PutUint16(setPixelFormat[8:10], 255)  // red-max
	binary.BigEndian.PutUint16(setPixelFormat[10:12], 255) // green-max
	binary.BigEndian.PutUint16(setPixelFormat[12:14], 255) // blue-max
	setPixelFormat[14] = 16                                // red-shift (BGRA: blue=0, green=8, red=16)
	setPixelFormat[15] = 8                                 // green-shift
	setPixelFormat[16] = 0                                 // blue-shift
	// bytes 17-19: padding

	if _, err := conn.Write(setPixelFormat); err != nil {
		return nil, fmt.Errorf("failed to send SetPixelFormat: %w", err)
	}

	// Step 9: Send SetEncodings (Raw encoding only)
	setEncodings := make([]byte, 8)
	setEncodings[0] = 2 // message-type: SetEncodings
	// byte 1: padding
	binary.BigEndian.PutUint16(setEncodings[2:4], 1) // number-of-encodings
	binary.BigEndian.PutUint32(setEncodings[4:8], 0) // Raw encoding = 0

	if _, err := conn.Write(setEncodings); err != nil {
		return nil, fmt.Errorf("failed to send SetEncodings: %w", err)
	}

	// Step 10: Send FramebufferUpdateRequest for full screen
	fbUpdateReq := make([]byte, 10)
	fbUpdateReq[0] = 3                                      // message-type: FramebufferUpdateRequest
	fbUpdateReq[1] = 0                                      // incremental: 0 (full update)
	binary.BigEndian.PutUint16(fbUpdateReq[2:4], 0)         // x-position
	binary.BigEndian.PutUint16(fbUpdateReq[4:6], 0)         // y-position
	binary.BigEndian.PutUint16(fbUpdateReq[6:8], fbWidth)   // width
	binary.BigEndian.PutUint16(fbUpdateReq[8:10], fbHeight) // height

	if _, err := conn.Write(fbUpdateReq); err != nil {
		return nil, fmt.Errorf("failed to send FramebufferUpdateRequest: %w", err)
	}

	// Step 11: Read FramebufferUpdate response
	// We may receive other message types; loop until we get type 0 (FramebufferUpdate)
	img := image.NewRGBA(image.Rect(0, 0, int(fbWidth), int(fbHeight)))

	for {
		// Check context
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Read message type
		msgType := make([]byte, 1)
		if _, err := readFull(conn, msgType); err != nil {
			return nil, fmt.Errorf("failed to read message type: %w", err)
		}

		switch msgType[0] {
		case 0: // FramebufferUpdate
			// padding(1) + num_rects(2)
			header := make([]byte, 3)
			if _, err := readFull(conn, header); err != nil {
				return nil, fmt.Errorf("failed to read FramebufferUpdate header: %w", err)
			}
			numRects := binary.BigEndian.Uint16(header[1:3])

			// Step 12: Read each rectangle
			for i := uint16(0); i < numRects; i++ {
				// x(2) + y(2) + w(2) + h(2) + encoding(4) = 12 bytes
				rectHeader := make([]byte, 12)
				if _, err := readFull(conn, rectHeader); err != nil {
					return nil, fmt.Errorf("failed to read rect header: %w", err)
				}

				rx := binary.BigEndian.Uint16(rectHeader[0:2])
				ry := binary.BigEndian.Uint16(rectHeader[2:4])
				rw := binary.BigEndian.Uint16(rectHeader[4:6])
				rh := binary.BigEndian.Uint16(rectHeader[6:8])
				encoding := binary.BigEndian.Uint32(rectHeader[8:12])

				if encoding != 0 {
					return nil, fmt.Errorf("unsupported encoding type: %d (only Raw encoding is supported)", encoding)
				}

				// Read pixel data (w * h * 4 bytes for 32bpp)
				pixelDataSize := int(rw) * int(rh) * 4
				pixelData := make([]byte, pixelDataSize)
				if _, err := readFull(conn, pixelData); err != nil {
					return nil, fmt.Errorf("failed to read pixel data: %w", err)
				}

				// Step 13: Convert BGRA pixel data to RGBA and write to image
				for py := 0; py < int(rh); py++ {
					for px := 0; px < int(rw); px++ {
						srcIdx := (py*int(rw) + px) * 4
						// With our pixel format (red-shift=16, green-shift=8, blue-shift=0):
						// The pixel data is in BGRX order in memory (little-endian)
						b := pixelData[srcIdx+0]
						g := pixelData[srcIdx+1]
						r := pixelData[srcIdx+2]
						a := uint8(255)

						imgX := int(rx) + px
						imgY := int(ry) + py
						if imgX < int(fbWidth) && imgY < int(fbHeight) {
							offset := (imgY-img.Rect.Min.Y)*img.Stride + (imgX-img.Rect.Min.X)*4
							img.Pix[offset+0] = r
							img.Pix[offset+1] = g
							img.Pix[offset+2] = b
							img.Pix[offset+3] = a
						}
					}
				}
			}

			// Successfully captured the framebuffer
			return img, nil

		case 1: // SetColourMapEntries - skip
			skipHeader := make([]byte, 5)
			if _, err := readFull(conn, skipHeader); err != nil {
				return nil, fmt.Errorf("failed to skip SetColourMapEntries header: %w", err)
			}
			numColors := binary.BigEndian.Uint16(skipHeader[3:5])
			skipData := make([]byte, int(numColors)*6)
			if _, err := readFull(conn, skipData); err != nil {
				return nil, fmt.Errorf("failed to skip SetColourMapEntries data: %w", err)
			}

		case 2: // Bell - no data to read
			continue

		case 3: // ServerCutText
			skipHeader := make([]byte, 7)
			if _, err := readFull(conn, skipHeader); err != nil {
				return nil, fmt.Errorf("failed to skip ServerCutText header: %w", err)
			}
			textLen := binary.BigEndian.Uint32(skipHeader[3:7])
			skipData := make([]byte, textLen)
			if _, err := readFull(conn, skipData); err != nil {
				return nil, fmt.Errorf("failed to skip ServerCutText data: %w", err)
			}

		default:
			return nil, fmt.Errorf("unexpected VNC message type: %d", msgType[0])
		}
	}
}

// SaveScreenshotPNG captures a screenshot and saves it as a PNG file.
func SaveScreenshotPNG(ctx context.Context, host string, port int, outputPath string) error {
	img, err := CaptureScreenshot(ctx, host, port)
	if err != nil {
		return fmt.Errorf("failed to capture screenshot: %w", err)
	}

	// Create parent directories if needed
	if dir := filepath.Dir(outputPath); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("failed to encode PNG: %w", err)
	}

	return nil
}

// readFull reads exactly len(buf) bytes from conn, handling partial reads.
func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
