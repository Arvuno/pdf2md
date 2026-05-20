// Package layout provides document layout detection using an ONNX PP-DocLayout model.
package layout

import (
	"fmt"
	"image"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

// ModelRepoID is the HuggingFace repository for the ONNX layout model.
const ModelRepoID = "alex-dinh/PP-DocLayoutV3-ONNX"

// ModelFileName is the ONNX model file to use.
const ModelFileName = "PP-DocLayoutV3.onnx"

// ConfigFileName is the JSON config with label list.
const ConfigFileName = "config.json"

// ModelInputWidth and ModelInputHeight are the fixed input size.
const (
	ModelInputWidth  = 800
	ModelInputHeight = 800
)

// ImageNet normalization parameters.
var (
	imagenetMean = [3]float32{0.485, 0.456, 0.406}
	imagenetStd  = [3]float32{0.229, 0.224, 0.225}
)

// Block is a detected layout element.
type Block struct {
	Label      string
	Confidence float32
	BBox       [4]float64 // x1, y1, x2, y2 in original image pixel coordinates
	ReadOrder  int
}

// Detector runs ONNX PP-DocLayout inference.
type Detector struct {
	session *ort.DynamicAdvancedSession
	labels  []string
}

// NewDetector creates a Detector from a model directory containing the .onnx file and config.json.
func NewDetector(modelDir string) (*Detector, error) {
	if !ort.IsInitialized() {
		return nil, fmt.Errorf("ONNX Runtime is not initialized")
	}

	modelPath := filepath.Join(modelDir, ModelFileName)
	cfgPath := filepath.Join(modelDir, ConfigFileName)

	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("layout model not found at %s: %w", modelPath, err)
	}

	labels, err := loadLabels(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading labels: %w", err)
	}

	options, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("creating session options: %w", err)
	}
	defer options.Destroy()

	session, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{"im_shape", "image", "scale_factor"},
		[]string{"fetch_name_0", "fetch_name_1", "fetch_name_2"},
		options)
	if err != nil {
		return nil, fmt.Errorf("creating ONNX session: %w", err)
	}

	return &Detector{session: session, labels: labels}, nil
}

// Detect runs layout detection on an image, returning detected blocks sorted by reading order.
func (d *Detector) Detect(img image.Image) ([]Block, error) {
	bounds := img.Bounds()
	origW, origH := bounds.Dx(), bounds.Dy()
	scaleW := float32(ModelInputWidth) / float32(origW)
	scaleH := float32(ModelInputHeight) / float32(origH)

	// Preprocess image to 800x800 CHW float32 tensor.
	inputData := preprocessImage(img, origW, origH)

	// Create input tensors.
	shapeTensor, err := ort.NewTensor(ort.NewShape(1, 2), []float32{float32(ModelInputHeight), float32(ModelInputWidth)})
	if err != nil {
		return nil, fmt.Errorf("creating shape tensor: %w", err)
	}
	defer shapeTensor.Destroy()

	imgTensor, err := ort.NewTensor(ort.NewShape(1, 3, ModelInputHeight, ModelInputWidth), inputData)
	if err != nil {
		return nil, fmt.Errorf("creating image tensor: %w", err)
	}
	defer imgTensor.Destroy()

	scaleTensor, err := ort.NewTensor(ort.NewShape(1, 2), []float32{scaleH, scaleW})
	if err != nil {
		return nil, fmt.Errorf("creating scale tensor: %w", err)
	}
	defer scaleTensor.Destroy()

	// Create output tensors.
	boxOut, err := ort.NewEmptyTensor[float32](ort.NewShape(300, 7))
	if err != nil {
		return nil, fmt.Errorf("creating box output: %w", err)
	}
	defer boxOut.Destroy()

	idxOut, err := ort.NewEmptyTensor[int32](ort.NewShape(1))
	if err != nil {
		return nil, fmt.Errorf("creating idx output: %w", err)
	}
	defer idxOut.Destroy()

	maskOut, err := ort.NewEmptyTensor[int32](ort.NewShape(300, 200, 200))
	if err != nil {
		return nil, fmt.Errorf("creating mask output: %w", err)
	}
	defer maskOut.Destroy()

	// Run inference.
	err = d.session.Run(
		[]ort.Value{shapeTensor, imgTensor, scaleTensor},
		[]ort.Value{boxOut, idxOut, maskOut},
	)
	if err != nil {
		return nil, fmt.Errorf("running ONNX inference: %w", err)
	}

	// Postprocess boxes: [label_idx, score, x1, y1, x2, y2, read_order]
	rawBoxes := boxOut.GetData()
	blocks := postprocessBoxes(rawBoxes, d.labels, scaleW, scaleH, 0.5)

	return blocks, nil
}

// Destroy releases ONNX resources.
func (d *Detector) Destroy() {
	if d.session != nil {
		d.session.Destroy()
	}
}

// InitONNXRuntime searches for libonnxruntime.so and initializes the ONNX runtime.
// Returns an error with installation instructions if not found.
func InitONNXRuntime() error {
	libPath := findONNXRuntimeLib()
	if libPath == "" {
		return fmt.Errorf(`
onnxruntime shared library (libonnxruntime.so) not found.

Install via one of these methods:

  # Option 1: apt (Ubuntu/Debian)
  sudo apt install libonnxruntime-dev

  # Option 2: manual download
  wget https://github.com/microsoft/onnxruntime/releases/download/v1.20.0/onnxruntime-linux-x64-gpu-1.20.0.tgz
  tar xzf onnxruntime-linux-x64-gpu-1.20.0.tgz
  sudo cp onnxruntime-linux-x64-gpu-1.20.0/lib/libonnxruntime.so* /usr/local/lib/
  sudo ldconfig

  # Option 3: if you have Python onnxruntime installed
  find ~/.local/lib/python*/site-packages/onnxruntime -name "libonnxruntime.so*" -exec sudo cp {} /usr/local/lib/ \;
  sudo ldconfig`)
	}

	ort.SetSharedLibraryPath(libPath)
	return ort.InitializeEnvironment()
}

// findONNXRuntimeLib searches common locations for libonnxruntime.so.
func findONNXRuntimeLib() string {
	candidates := []string{
		"libonnxruntime.so",
		"/usr/lib/libonnxruntime.so",
		"/usr/local/lib/libonnxruntime.so",
		"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
	}

	// Try system-wide locations.
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Try Python onnxruntime paths.
	for _, pyVer := range []string{"3.13", "3.12", "3.11", "3.10", "3.9", "3.8"} {
		for _, base := range []string{os.Getenv("HOME") + "/.local", "/usr/local", "/usr", os.Getenv("HOME") + "/miniforge3"} {
			for _, libDir := range []string{"lib/python" + pyVer, "lib64/python" + pyVer, "Library/lib"} {
				matches, _ := filepath.Glob(filepath.Join(base, libDir, "site-packages/onnxruntime/capi/libonnxruntime.so*"))
				for _, m := range matches {
					if !strings.HasSuffix(m, ".py") && !strings.HasSuffix(m, ".pyc") {
						if _, err := os.Stat(m); err == nil {
							return m
						}
					}
				}
			}
		}
	}

	// Try ldconfig cache.
	if out, err := exec.Command("ldconfig", "-p").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "libonnxruntime.so") {
				fields := strings.Fields(line)
				if len(fields) >= 4 {
					return fields[len(fields)-1]
				}
			}
		}
	}

	// Try runtime platform defaults.
	if runtime.GOOS == "linux" {
		for _, p := range []string{"/usr/lib/libonnxruntime.so.1", "/usr/local/lib/libonnxruntime.so.1"} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	return ""
}

// preprocessImage converts a Go image.Image to a [1, 3, 800, 800] float32 tensor
// with ImageNet normalization.
func preprocessImage(img image.Image, origW, origH int) []float32 {
	targetW, targetH := ModelInputWidth, ModelInputHeight
	data := make([]float32, 3*targetW*targetH)

	ri, gi, bi := 0, targetW*targetH, 2*targetW*targetH

	for y := 0; y < targetH; y++ {
		srcY := int(float64(y) * float64(origH) / float64(targetH))
		for x := 0; x < targetW; x++ {
			srcX := int(float64(x) * float64(origW) / float64(targetW))
			r, g, b, _ := img.At(srcX, srcY).RGBA()
			data[ri] = (float32(r>>8)/255.0 - imagenetMean[0]) / imagenetStd[0]
			data[gi] = (float32(g>>8)/255.0 - imagenetMean[1]) / imagenetStd[1]
			data[bi] = (float32(b>>8)/255.0 - imagenetMean[2]) / imagenetStd[2]
			ri++
			gi++
			bi++
		}
	}

	return data
}

// postprocessBoxes converts raw ONNX output to layout Blocks.
func postprocessBoxes(raw []float32, labels []string, scaleW, scaleH float32, confThresh float32) []Block {
	var blocks []Block

	for i := 0; i+7 <= len(raw); i += 7 {
		labelIdx := int(raw[i])
		score := raw[i+1]
		x1 := raw[i+2]
		y1 := raw[i+3]
		x2 := raw[i+4]
		y2 := raw[i+5]
		readOrder := int(raw[i+6])

		if score < confThresh {
			continue
		}
		if labelIdx < 0 || labelIdx >= len(labels) {
			continue
		}

		// Map coordinates from 800x800 space back to original image space.
		// x_orig = x / scale_w, y_orig = y / scale_h
		// where scale_w = 800 / orig_w, scale_h = 800 / orig_h
		ox1 := float64(x1 / scaleW)
		oy1 := float64(y1 / scaleH)
		ox2 := float64(x2 / scaleW)
		oy2 := float64(y2 / scaleH)

		// Clamp to reasonable bounds.
		ox1 = math.Max(0, ox1)
		oy1 = math.Max(0, oy1)

		blocks = append(blocks, Block{
			Label:      labels[labelIdx],
			Confidence: score,
			BBox:       [4]float64{ox1, oy1, ox2, oy2},
			ReadOrder:  readOrder,
		})
	}

	// Sort by reading order, then by position as fallback.
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].ReadOrder != 0 || blocks[j].ReadOrder != 0 {
			return blocks[i].ReadOrder < blocks[j].ReadOrder
		}
		if blocks[i].BBox[1] == blocks[j].BBox[1] {
			return blocks[i].BBox[0] < blocks[j].BBox[0]
		}
		return blocks[i].BBox[1] < blocks[j].BBox[1]
	})

	return blocks
}
