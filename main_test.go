package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsImageOnlyDir(t *testing.T) {
	tempDir := t.TempDir()

	// Case 1: Empty directory
	isImage, err := isImageOnlyDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to check empty dir: %v", err)
	}
	if isImage {
		t.Error("Empty directory should not be considered an image-only directory")
	}

	// Case 2: Only images
	imageDir := filepath.Join(tempDir, "images")
	os.Mkdir(imageDir, 0755)
	os.WriteFile(filepath.Join(imageDir, "1.jpg"), []byte("fake image"), 0644)
	os.WriteFile(filepath.Join(imageDir, "2.png"), []byte("fake image"), 0644)

	isImage, err = isImageOnlyDir(imageDir)
	if err != nil {
		t.Fatalf("Failed to check image dir: %v", err)
	}
	if !isImage {
		t.Error("Directory with only images should be recognized")
	}

	// Case 3: Mixed with other files (not archives or subdirs)
	os.WriteFile(filepath.Join(imageDir, "readme.txt"), []byte("not an image"), 0644)
	isImage, err = isImageOnlyDir(imageDir)
	if err != nil {
		t.Fatalf("Failed to check mixed dir: %v", err)
	}
	if !isImage {
		t.Error("Directory with images and other non-conflict files should still be recognized (as per current implementation)")
	}

	// Case 4: Contains subdirectories
	subDir := filepath.Join(imageDir, "subdir")
	os.Mkdir(subDir, 0755)
	isImage, err = isImageOnlyDir(imageDir)
	if err != nil {
		t.Fatalf("Failed to check dir with subdir: %v", err)
	}
	if isImage {
		t.Error("Directory with subdirectories should NOT be recognized")
	}
	os.RemoveAll(subDir)

	// Case 5: Contains archives
	os.WriteFile(filepath.Join(imageDir, "test.zip"), []byte("fake zip"), 0644)
	isImage, err = isImageOnlyDir(imageDir)
	if err != nil {
		t.Fatalf("Failed to check dir with archive: %v", err)
	}
	if isImage {
		t.Error("Directory with archives should NOT be recognized")
	}
}

func TestResolveRarPath(t *testing.T) {
	// Test user specified path
	customPath := "C:\\custom\\rar.exe"
	if resolveRarPath(customPath) != customPath {
		t.Errorf("Expected custom path %s, got %s", customPath, resolveRarPath(customPath))
	}

	// Test default path (this might be flaky across machines but should return SOMETHING)
	path := resolveRarPath("")
	if path == "" {
		t.Error("Expected a non-empty RAR path")
	}
}

func TestArchiveWorkflow(t *testing.T) {
	// Check if 7z is available
	_, err := exec.LookPath("7z")
	if err != nil {
		t.Skip("7z not found in PATH, skipping workflow tests")
	}

	tempDir := t.TempDir()
	chapterName := "Chapter 01"
	chapterDir := filepath.Join(tempDir, chapterName)
	os.Mkdir(chapterDir, 0755)

	// Add some dummy images
	os.WriteFile(filepath.Join(chapterDir, "01.jpg"), []byte("dummy image 1"), 0644)
	os.WriteFile(filepath.Join(chapterDir, "02.jpg"), []byte("dummy image 2"), 0644)

	formats := []string{"zip", "7z"}

	// Test RAR if available
	rarPath := resolveRarPath("")
	if _, err := os.Stat(rarPath); err == nil || rarPath == "rar" {
		// We can't be 100% sure "rar" is in PATH if it's just a string, 
		// but createArchive will fail gracefully if it isn't.
		// Let's only add it if we are fairly sure.
		if _, err := exec.LookPath("rar"); err == nil {
			formats = append(formats, "rar")
		}
	}

	for _, format := range formats {
		t.Run("Format_"+format, func(t *testing.T) {
			// Create a new temp dir for each format to avoid conflicts
			workDir := t.TempDir()
			targetChapterDir := filepath.Join(workDir, chapterName)
			os.Mkdir(targetChapterDir, 0755)
			os.WriteFile(filepath.Join(targetChapterDir, "01.jpg"), []byte("dummy image 1"), 0644)

			// We use processChapter because it covers create, verify AND cleanup
			err := processChapter(targetChapterDir, format, 10, "", true)
			if err != nil {
				t.Fatalf("Workflow failed for %s: %v", format, err)
			}

			// Verify cleanup
			if _, err := os.Stat(targetChapterDir); !os.IsNotExist(err) {
				t.Errorf("Source directory %s was not deleted after successful processing", targetChapterDir)
			}

			// Verify archive exists
			archiveName := targetChapterDir + "." + format
			if _, err := os.Stat(archiveName); os.IsNotExist(err) {
				t.Errorf("Archive %s was not created", archiveName)
			}
		})
	}
}

func TestProgress(t *testing.T) {
	p := &Progress{totalChapters: 10}
	p.increment(true)
	if p.processedCount != 1 || p.failedCount != 0 {
		t.Errorf("Progress increment failed: got processed=%d, failed=%d", p.processedCount, p.failedCount)
	}
	p.increment(false)
	if p.processedCount != 2 || p.failedCount != 1 {
		t.Errorf("Progress increment failure tracking failed: got processed=%d, failed=%d", p.processedCount, p.failedCount)
	}
}
