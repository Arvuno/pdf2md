package paddlelayout

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseObjectFromFence(t *testing.T) {
	raw := "```json\n{\"blocks\":[{\"bbox\":[10,20,30,40],\"label\":\"title\",\"text\":\"Hello\",\"order\":2},{\"bbox\":[0,0,5,5],\"label\":\"text\",\"text\":\"First\",\"order\":1}]}\n```"
	res, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(res.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(res.Blocks))
	}
	if res.Blocks[0].Text != "First" {
		t.Errorf("expected blocks sorted by order, got %+v", res.Blocks)
	}
}

func TestToMarkdown(t *testing.T) {
	md := ToMarkdown([]Block{
		{Label: "title", Text: "Doc"},
		{Label: "formula", Text: "x=1"},
		{Label: "image", CropImage: "page_blocks/block-001_image.png"},
	})
	for _, want := range []string{"# Doc", "$$\nx=1\n$$", "![image](page_blocks/block-001_image.png)"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestSaveCrops(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "page.png")
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatalf("Encode failed: %v", err)
	}
	f.Close()

	blocks, err := SaveCrops(imgPath, filepath.Join(dir, "blocks"), []Block{{BBox: [4]float64{10, 10, 40, 40}, Label: "Text"}})
	if err != nil {
		t.Fatalf("SaveCrops failed: %v", err)
	}
	if len(blocks) != 1 || blocks[0].CropImage == "" {
		t.Fatalf("expected crop image, got %+v", blocks)
	}
	if _, err := os.Stat(blocks[0].CropImage); err != nil {
		t.Fatalf("crop image missing: %v", err)
	}
}

func TestFallbackPageBlock(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "page.png")
	img := image.NewRGBA(image.Rect(0, 0, 20, 30))
	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatalf("Encode failed: %v", err)
	}
	f.Close()

	block, err := FallbackPageBlock(imgPath, "hello")
	if err != nil {
		t.Fatalf("FallbackPageBlock failed: %v", err)
	}
	if block.Label != "page" || block.Text != "hello" || block.BBox != [4]float64{0, 0, 20, 30} {
		t.Fatalf("unexpected block: %+v", block)
	}
}
