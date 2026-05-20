//go:build cgo

package layout

// Package layout provides document layout detection using an ONNX PP-DocLayout model.

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	ort "github.com/yalue/onnxruntime_go"
)

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

// Detector runs ONNX PP-DocLayout inference.
type Detector struct {
	session *ort.DynamicAdvancedSession
	labels  []string
}

// NewDetector creates a Detector from a model directory.
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

	// Try CUDA first for GPU acceleration; warn prominently if CPU fallback.
	cudaOpts, cudaErr := ort.NewCUDAProviderOptions()
	if cudaErr == nil {
		cudaOpts.Update(map[string]string{"device_id": "0"})
		cudaErr = options.AppendExecutionProviderCUDA(cudaOpts)
		cudaOpts.Destroy()
	}
	if cudaErr != nil {
		fmt.Fprintf(os.Stderr, "\n⚠️  WARNING: ONNX CUDA unavailable (%v), falling back to CPU.\n   Layout detection will be slow.\n   Ensure libcudnn.so is in LD_LIBRARY_PATH.\n\n", cudaErr)
	}

	session, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{"im_shape", "image", "scale_factor"},
		[]string{"fetch_name_0", "fetch_name_1", "fetch_name_2"},
		options)
	if err != nil {
		return nil, fmt.Errorf("creating ONNX session: %w", err)
	}

	return &Detector{session: session, labels: labels}, nil
}

// Detect runs layout detection on an image.
func (d *Detector) Detect(img image.Image) ([]Block, error) {
	bounds := img.Bounds()
	origW, origH := bounds.Dx(), bounds.Dy()
	scaleW := float32(ModelInputWidth) / float32(origW)
	scaleH := float32(ModelInputHeight) / float32(origH)

	inputData := preprocessImage(img, origW, origH)

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

	err = d.session.Run(
		[]ort.Value{shapeTensor, imgTensor, scaleTensor},
		[]ort.Value{boxOut, idxOut, maskOut},
	)
	if err != nil {
		return nil, fmt.Errorf("running ONNX inference: %w", err)
	}

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

// InitONNXRuntime searches for libonnxruntime.so, sets up CUDA paths, and initializes.
func InitONNXRuntime() error {
	libPath := findONNXRuntimeLib()
	if libPath == "" {
		return missingLibError()
	}

	// Symlink cuDNN into ORT dir.
	ortDir := filepath.Dir(libPath)
	cudnnDir := findCUDNNLib(ortDir)
	if cudnnDir != "" {
		matches, _ := filepath.Glob(filepath.Join(cudnnDir, "libcudnn*.so*"))
		for _, src := range matches {
			dst := filepath.Join(ortDir, filepath.Base(src))
			if _, err := os.Stat(dst); os.IsNotExist(err) {
				_ = os.Symlink(src, dst)
			}
		}
		// CUDA provider dlopen needs libcudnn in the linker search path.
		// os.Setenv doesn't affect the current process, so we re-exec with
		// LD_LIBRARY_PATH set correctly. The _ORT_CUDA_FIXED guard prevents
		// infinite re-exec loops.
		if os.Getenv("_ORT_CUDA_FIXED") != "1" {
			cur := os.Getenv("LD_LIBRARY_PATH")
			newPath := ortDir
			if cur != "" {
				newPath = ortDir + ":" + cur
			}
			os.Setenv("LD_LIBRARY_PATH", newPath)
			os.Setenv("_ORT_CUDA_FIXED", "1")
			if exe, err := os.Executable(); err == nil {
				syscall.Exec(exe, os.Args, os.Environ())
			}
		}
	}

	ort.SetSharedLibraryPath(libPath)
	return ort.InitializeEnvironment()
}

func missingLibError() error {
	return fmt.Errorf(`
onnxruntime shared library (libonnxruntime.so) not found.

Install via:

  # Option 1: pip
  pip install onnxruntime-gpu

  # Option 2: apt
  sudo apt install libonnxruntime-dev`)
}

// findONNXRuntimeLib searches common locations for libonnxruntime.so.
func findONNXRuntimeLib() string {
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

	// Try system paths.
	for _, p := range []string{
		"/usr/lib/libonnxruntime.so",
		"/usr/local/lib/libonnxruntime.so",
		"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Try Python onnxruntime paths.
	for _, pyVer := range []string{"3.13", "3.12", "3.11", "3.10", "3.9", "3.8"} {
		for _, base := range []string{os.Getenv("HOME") + "/.local", "/usr/local", "/usr", os.Getenv("HOME") + "/miniforge3"} {
			for _, libDir := range []string{"lib/python" + pyVer, "lib64/python" + pyVer} {
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

	return ""
}

// findCUDNNLib searches for libcudnn.so near the onnxruntime lib dir.
func findCUDNNLib(ortLibDir string) string {
	for _, base := range []string{
		filepath.Dir(ortLibDir),               // site-packages/onnxruntime → site-packages
		filepath.Dir(filepath.Dir(ortLibDir)), // → lib/pythonX.X
	} {
		for _, sub := range []string{"nvidia/cudnn/lib"} {
			cand := filepath.Join(base, sub)
			if matches, _ := filepath.Glob(filepath.Join(cand, "libcudnn.so*")); len(matches) > 0 {
				return cand
			}
		}
	}
	return ""
}

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

		// Model internally maps coordinates to original image space.
		blocks = append(blocks, Block{
			Label:      labels[labelIdx],
			Confidence: score,
			BBox:       [4]float64{float64(x1), float64(y1), float64(x2), float64(y2)},
			ReadOrder:  readOrder,
		})
	}

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
