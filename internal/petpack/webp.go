// Package petpack owns the external animated pet artwork format. It keeps
// WebP parsing and decoding out of internal/pet, so the pet model continues
// to depend only on MyGo's ui package and the standard library.
package petpack

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
	stddraw "image/draw"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	// maxCanvasEdge bounds allocations made from untrusted WebP headers.
	// A 4096×4096 RGBA canvas is 64 MiB; larger artwork is not useful for a
	// desktop pet and a malformed file must not be able to exhaust memory.
	maxCanvasEdge = 4096
	// maxClipFrames bounds the metadata retained for one clip. The largest
	// authored pack has far fewer frames; this catches corrupt ANMF streams.
	maxClipFrames = 1000
)

// FrameInfo describes one animation frame. Rect is in canvas pixels and
// Duration is the time the frame stays visible.
type FrameInfo struct {
	Rect     image.Rectangle
	Duration time.Duration
	Blend    bool
	Dispose  bool

	chunks []chunkRef
}

type chunkRef struct {
	id     [4]byte
	offset int
	length int
}

// Clip is one animation's metadata. The compressed bytes are loaded by a
// Decoder on demand and released when that decoder closes, so a pack with
// many clips does not keep all artwork in memory at once.
type Clip struct {
	ID            string
	Width, Height int
	Frames        []FrameInfo
	Crop          image.Rectangle
	Loop          bool

	fsys      fs.FS
	name      string
	data      []byte
	digest    [sha256.Size]byte
	hasDigest bool
}

// OpenClip reads and parses one animated WebP file from disk.
func OpenClip(id, path string) (*Clip, error) {
	return OpenClipFS(id, os.DirFS(filepath.Dir(path)), filepath.Base(path))
}

// OpenClipFS reads and parses one animated WebP file from a filesystem. The
// same loader serves installed packs and the embedded built-in pack.
func OpenClipFS(id string, fsys fs.FS, name string) (*Clip, error) {
	if fsys == nil {
		return nil, fmt.Errorf("petpack %q: nil filesystem", id)
	}
	if !fs.ValidPath(name) {
		return nil, fmt.Errorf("petpack %q: invalid clip path %q", id, name)
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	clip, err := ParseClip(id, data)
	if err != nil {
		return nil, err
	}
	clip.fsys = fsys
	clip.name = name
	return clip, nil
}

// ParseClip parses an animated WebP container. It accepts the lossless
// VP8L frame chunks used by the naiwa pack and the lossy ALPH+VP8 form.
func ParseClip(id string, data []byte) (*Clip, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, fmt.Errorf("petpack %q: not a WebP RIFF file", id)
	}
	clip := &Clip{ID: id, data: data}
	for pos := 12; pos+8 <= len(data); {
		name := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		if size < 0 || size > len(data)-pos-8 {
			return nil, fmt.Errorf("petpack %q: %s chunk exceeds the file", id, name)
		}
		body := data[pos+8 : pos+8+size]
		switch name {
		case "VP8X":
			if len(body) < 10 {
				return nil, fmt.Errorf("petpack %q: short VP8X chunk", id)
			}
			clip.Width = int(u24(body[4:7])) + 1
			clip.Height = int(u24(body[7:10])) + 1
			if err := validateCanvasSize(id, clip.Width, clip.Height); err != nil {
				return nil, err
			}
		case "ANMF":
			if len(clip.Frames) >= maxClipFrames {
				return nil, fmt.Errorf("petpack %q: more than %d frames", id, maxClipFrames)
			}
			frame, err := parseFrame(id, body, pos+8)
			if err != nil {
				return nil, err
			}
			clip.Frames = append(clip.Frames, frame)
		}
		pos += 8 + size
		if size%2 == 1 {
			pos++
		}
	}
	if clip.Width <= 0 || clip.Height <= 0 {
		return nil, fmt.Errorf("petpack %q: missing VP8X canvas", id)
	}
	if err := validateCanvasSize(id, clip.Width, clip.Height); err != nil {
		return nil, err
	}
	if len(clip.Frames) == 0 {
		return nil, fmt.Errorf("petpack %q: no animation frames", id)
	}
	canvas := image.Rect(0, 0, clip.Width, clip.Height)
	for i, frame := range clip.Frames {
		if frame.Rect.Dx() <= 0 || frame.Rect.Dy() <= 0 || !frame.Rect.In(canvas) {
			return nil, fmt.Errorf("petpack %q: frame %d rect %v outside canvas %v", id, i, frame.Rect, canvas)
		}
	}
	clip.Crop = canvas
	return clip, nil
}

func validateCanvasSize(id string, width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("petpack %q: invalid canvas %dx%d", id, width, height)
	}
	if width > maxCanvasEdge || height > maxCanvasEdge {
		return fmt.Errorf("petpack %q: canvas %dx%d exceeds the %dpx limit", id, width, height, maxCanvasEdge)
	}
	return nil
}

func parseFrame(id string, body []byte, bodyOffset int) (FrameInfo, error) {
	if len(body) < 16 {
		return FrameInfo{}, fmt.Errorf("petpack %q: short ANMF frame", id)
	}
	x := u24(body[0:3]) * 2
	y := u24(body[3:6]) * 2
	width := u24(body[6:9]) + 1
	height := u24(body[9:12]) + 1
	durationMS := u24(body[12:15])
	flags := body[15]
	frame := FrameInfo{
		Rect:     image.Rect(x, y, x+width, y+height),
		Duration: time.Duration(durationMS) * time.Millisecond,
		Dispose:  flags&1 != 0,
		Blend:    flags&2 == 0,
	}
	for pos := 16; pos+8 <= len(body); {
		length := int(binary.LittleEndian.Uint32(body[pos+4 : pos+8]))
		if length < 0 || pos+8+length > len(body) {
			return FrameInfo{}, fmt.Errorf("petpack %q: frame chunk exceeds ANMF", id)
		}
		var chunk chunkRef
		copy(chunk.id[:], body[pos:pos+4])
		chunk.offset = bodyOffset + pos + 8
		chunk.length = length
		frame.chunks = append(frame.chunks, chunk)
		pos += 8 + length + length%2
	}
	if err := validateFrameChunks(id, frame); err != nil {
		return FrameInfo{}, err
	}
	return frame, nil
}

func validateFrameChunks(id string, frame FrameInfo) error {
	var alpha, lossy, lossless bool
	for _, chunk := range frame.chunks {
		switch string(chunk.id[:]) {
		case "ALPH":
			alpha = true
		case "VP8 ":
			lossy = true
		case "VP8L":
			lossless = true
		default:
			// The WebP container permits unknown chunks inside a frame;
			// decoders must ignore them rather than reject the file.
			continue
		}
	}
	if lossless && (lossy || alpha) {
		return fmt.Errorf("petpack %q: frame mixes VP8L with ALPH or VP8", id)
	}
	if !lossless && !lossy {
		return fmt.Errorf("petpack %q: frame has no image chunk", id)
	}
	if alpha && !lossy {
		return fmt.Errorf("petpack %q: ALPH requires a VP8 frame", id)
	}
	return nil
}

// FrameCount returns the number of frames in the clip.
func (c *Clip) FrameCount() int { return len(c.Frames) }

// Duration returns one frame's display time.
func (c *Clip) Duration(i int) time.Duration {
	if i < 0 || i >= len(c.Frames) {
		return 0
	}
	return c.Frames[i].Duration
}

// TotalDuration sums every frame's display time.
func (c *Clip) TotalDuration() time.Duration {
	var total time.Duration
	for _, frame := range c.Frames {
		total += frame.Duration
	}
	return total
}

// Decoder composites frames onto one canvas in order. It is deliberately
// sequential: ANMF frames contain partial rectangles and rely on the previous
// canvas state, so random access would need a keyframe and a replay.
type Decoder struct {
	clip   *Clip
	data   []byte
	canvas *image.RGBA
	next   int
	last   image.Rectangle

	// Two scaled buffers keep the previous frame intact until the next
	// Update. ui.Bitmap.Update may hold the pixel slice until the following
	// call, so returning the same buffer every frame can race the upload.
	scaled     [2]*image.RGBA
	scaleIndex int
	scaleW     int
	scaleH     int
}

// NewDecoder starts a decoder at the first frame with a transparent canvas.
// It loads the clip's compressed bytes when the clip was opened from disk.
func (c *Clip) NewDecoder() (*Decoder, error) {
	if err := validateCanvasSize(c.ID, c.Width, c.Height); err != nil {
		return nil, err
	}
	data := c.data
	if data == nil {
		if c.fsys == nil || c.name == "" {
			return nil, fmt.Errorf("petpack %q: no backing file", c.ID)
		}
		var err error
		data, err = fs.ReadFile(c.fsys, c.name)
		if err != nil {
			return nil, fmt.Errorf("petpack %q: read clip: %w", c.ID, err)
		}
		if c.hasDigest && sha256.Sum256(data) != c.digest {
			return nil, fmt.Errorf("petpack %q: clip changed on disk since it was loaded; restart Lucia after re-importing", c.ID)
		}
	}
	return &Decoder{
		clip:   c,
		data:   data,
		canvas: image.NewRGBA(image.Rect(0, 0, c.Width, c.Height)),
	}, nil
}

// Close releases the decoder's compressed bytes and canvases. The clip
// metadata remains valid for a later NewDecoder.
func (d *Decoder) Close() {
	d.data = nil
	d.canvas = nil
	d.scaled[0] = nil
	d.scaled[1] = nil
	d.last = image.Rectangle{}
	d.next = 0
}

// Reset rewinds the decoder to the first frame and clears the canvas. The
// first frame of a valid loop is a full-canvas keyframe.
func (d *Decoder) Reset() {
	if d.canvas == nil {
		return
	}
	stddraw.Draw(d.canvas, d.canvas.Bounds(), image.Transparent, image.Point{}, stddraw.Src)
	d.next = 0
	d.last = image.Rectangle{}
}

// Next decodes and composites the next frame. The returned image is the
// decoder's canvas and is only valid until the next call. A decoded frame
// that is not ready yet never happens here; callers that prefetch can do so
// outside the UI thread.
func (d *Decoder) Next() (image.Image, time.Duration, error) {
	if d.canvas == nil || d.data == nil {
		return nil, 0, fmt.Errorf("petpack %q: decoder is closed", d.clip.ID)
	}
	if d.next >= len(d.clip.Frames) {
		return nil, 0, io.EOF
	}
	frame := &d.clip.Frames[d.next]
	if d.next > 0 && d.clip.Frames[d.next-1].Dispose {
		stddraw.Draw(d.canvas, d.last, image.Transparent, image.Point{}, stddraw.Src)
	}
	src, err := d.decodeFrame(frame)
	if err != nil {
		return nil, 0, err
	}
	bounds := src.Bounds()
	if bounds.Dx() != frame.Rect.Dx() || bounds.Dy() != frame.Rect.Dy() {
		return nil, 0, fmt.Errorf("petpack %q frame %d: decoded %dx%d, want %dx%d",
			d.clip.ID, d.next, bounds.Dx(), bounds.Dy(), frame.Rect.Dx(), frame.Rect.Dy())
	}
	if !frame.Rect.In(image.Rect(0, 0, d.clip.Width, d.clip.Height)) {
		return nil, 0, fmt.Errorf("petpack %q frame %d: rectangle %v outside canvas", d.clip.ID, d.next, frame.Rect)
	}
	op := stddraw.Src
	if frame.Blend {
		op = stddraw.Over
	}
	stddraw.Draw(d.canvas, frame.Rect, src, bounds.Min, op)
	d.last = frame.Rect
	d.next++
	return d.canvas, frame.Duration, nil
}

// NextScaled returns the current frame cropped to the pack's crop rectangle
// and scaled to width×height. It alternates between two buffers, so a returned
// image stays unchanged until the next two calls.
func (d *Decoder) NextScaled(width, height int) (image.Image, time.Duration, error) {
	img, duration, err := d.Next()
	if err != nil {
		return nil, 0, err
	}
	return d.cropAndScale(img, width, height), duration, nil
}

func (d *Decoder) cropAndScale(img image.Image, width, height int) image.Image {
	crop := d.clip.Crop
	if crop.Empty() {
		crop = image.Rect(0, 0, d.clip.Width, d.clip.Height)
	}
	src := img.(*image.RGBA).SubImage(crop)
	if width <= 0 || height <= 0 {
		return src
	}
	if d.scaleW != width || d.scaleH != height {
		d.scaled[0] = nil
		d.scaled[1] = nil
		d.scaleW, d.scaleH = width, height
	}
	index := d.scaleIndex
	d.scaleIndex ^= 1
	dst := d.scaled[index]
	if dst == nil {
		dst = image.NewRGBA(image.Rect(0, 0, width, height))
		d.scaled[index] = dst
	}
	if width == crop.Dx() && height == crop.Dy() {
		stddraw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, stddraw.Src)
		return dst
	}
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), stddraw.Src, nil)
	return dst
}

// DecodeAll decodes every frame at the crop's native size. Each returned
// image is an independent copy suitable for a long-lived cache.
func (c *Clip) DecodeAll() ([]*image.RGBA, error) {
	return c.decodeAll(0, 0)
}

// DecodeAllScaled decodes every frame at width×height. A zero size uses the
// crop's native size.
func (c *Clip) DecodeAllScaled(width, height int) ([]*image.RGBA, error) {
	return c.decodeAll(width, height)
}

func (c *Clip) decodeAll(width, height int) ([]*image.RGBA, error) {
	decoder, err := c.NewDecoder()
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	frames := make([]*image.RGBA, 0, len(c.Frames))
	for i := range c.Frames {
		img, _, err := decoder.Next()
		if err != nil {
			return nil, fmt.Errorf("decode frame %d: %w", i, err)
		}
		frames = append(frames, copyCropped(img, c.Crop, width, height))
	}
	return frames, nil
}

func copyCropped(img image.Image, crop image.Rectangle, width, height int) *image.RGBA {
	if crop.Empty() {
		crop = img.Bounds()
	}
	src := img.(*image.RGBA).SubImage(crop)
	w, h := crop.Dx(), crop.Dy()
	if width > 0 && height > 0 {
		w, h = width, height
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if w == crop.Dx() && h == crop.Dy() {
		stddraw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, stddraw.Src)
		return dst
	}
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), stddraw.Src, nil)
	return dst
}

// AlphaBounds returns the union of non-transparent pixels across every frame
// in canvas coordinates. Importers use it to crop away transparent margins.
func (c *Clip) AlphaBounds() (image.Rectangle, error) {
	decoder, err := c.NewDecoder()
	if err != nil {
		return image.Rectangle{}, err
	}
	defer decoder.Close()
	var union image.Rectangle
	for i := range c.Frames {
		img, _, err := decoder.Next()
		if err != nil {
			return image.Rectangle{}, fmt.Errorf("decode frame %d: %w", i, err)
		}
		rgba := img.(*image.RGBA)
		for y := 0; y < rgba.Rect.Dy(); y++ {
			row := rgba.Pix[y*rgba.Stride : y*rgba.Stride+rgba.Rect.Dx()*4]
			for x := 0; x < len(row); x += 4 {
				if row[x+3] != 0 {
					pixel := image.Rect(x/4, y, x/4+1, y+1)
					if union.Empty() {
						union = pixel
					} else {
						union = union.Union(pixel)
					}
				}
			}
		}
	}
	return union, nil
}

func (d *Decoder) decodeFrame(frame *FrameInfo) (image.Image, error) {
	wrapped, err := d.wrapFrame(frame)
	if err != nil {
		return nil, err
	}
	img, err := webp.Decode(bytes.NewReader(wrapped))
	if err != nil {
		return nil, fmt.Errorf("petpack %q: decode frame: %w", d.clip.ID, err)
	}
	return img, nil
}

func (d *Decoder) wrapFrame(frame *FrameInfo) ([]byte, error) {
	var body bytes.Buffer
	var alpha bool
	for _, chunk := range frame.chunks {
		if string(chunk.id[:]) == "ALPH" {
			alpha = true
			break
		}
	}
	if alpha {
		body.WriteString("VP8X")
		binary.Write(&body, binary.LittleEndian, uint32(10))
		body.WriteByte(1 << 4) // alpha
		body.Write([]byte{0, 0, 0})
		writeU24(&body, frame.Rect.Dx()-1)
		writeU24(&body, frame.Rect.Dy()-1)
	}
	for _, chunk := range frame.chunks {
		if chunk.offset < 0 || chunk.length < 0 ||
			chunk.offset > len(d.data) || chunk.length > len(d.data)-chunk.offset {
			return nil, fmt.Errorf("petpack %q: frame chunk outside clip data", d.clip.ID)
		}
		body.Write(chunk.id[:])
		binary.Write(&body, binary.LittleEndian, uint32(chunk.length))
		body.Write(d.data[chunk.offset : chunk.offset+chunk.length])
		if chunk.length%2 == 1 {
			body.WriteByte(0)
		}
	}
	var out bytes.Buffer
	out.WriteString("RIFF")
	binary.Write(&out, binary.LittleEndian, uint32(body.Len()+4))
	out.WriteString("WEBP")
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

func u24(b []byte) int {
	return int(b[0]) | int(b[1])<<8 | int(b[2])<<16
}

func writeU24(buf *bytes.Buffer, value int) {
	buf.Write([]byte{byte(value), byte(value >> 8), byte(value >> 16)})
}
