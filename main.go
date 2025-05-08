package main

import (
	"archive/zip"
	"compress/flate"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	rootDir := "C:/Users/saber/Downloads/dmzjDownload/comic" // 替换为你的漫画根目录路径
	//rootDir := "./comic" // 替换为你的漫画根目录路径

	pwd, err := os.Getwd()
	fmt.Println(pwd)

	//rootDir = filepath.Join(pwd, rootDir)

	// 收集所有章节目录
	var chapterMap = make(map[string]struct{})
	var chapterDirs []string

	dirCount := 0

	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			//return err
			log.Println(err)
		}

		if path != rootDir {

			stat, _ := os.Stat(path)
			if stat.IsDir() {
				dirCount++
			}

			ext := strings.ToUpper(filepath.Ext(path))
			if ext == ".JPG" || ext == ".JPEG" || ext == ".PNG" {
				chapterDir := filepath.Dir(path)

				if _, exists := chapterMap[chapterDir]; !exists {
					chapterMap[chapterDir] = struct{}{}
					chapterDirs = append(chapterDirs, chapterDir)
				}

			}

		}

		// 如果是文件夹，并且不是根目录
		//if info.IsDir() && path != rootDir {
		//	fmt.Printf("Processing directory: %s\n", path)
		//	err := processChapter(path)
		//	if err != nil {
		//		fmt.Printf("Error processing directory %s: %v\n", path, err)
		//	}
		//}
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking the directory: %v\n", err)
	}

	for _, chapterDir := range chapterDirs {
		err := processChapter(chapterDir)
		if err != nil {
			fmt.Printf("Error processing directory %s: %v\n", chapterDir, err)
		}
	}

}

// 处理单个章节文件夹
func processChapter(chapterDir string) error {
	zipFileName := chapterDir + ".zip"

	// 创建压缩包
	//err := createZip(chapterDir, zipFileName, flate.BestSpeed)
	err := createZipWith7Zip(chapterDir, zipFileName)
	if err != nil {
		return fmt.Errorf("failed to create zip: %w", err)
	}

	// 验证压缩包完整性
	err = testZipWith7Zip(zipFileName)
	if err != nil {
		return fmt.Errorf("zip integrity test failed: %w", err)
	}

	// 删除章节文件夹
	err = os.RemoveAll(chapterDir)
	if err != nil {
		return fmt.Errorf("failed to remove original directory: %w", err)
	}

	fmt.Printf("Successfully processed and removed directory: %s\n", chapterDir)
	return nil
}

// 创建 ZIP 文件
func createZip(sourceDir, zipFileName string, compressionLevel int) error {
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 注册自定义压缩器
	zipWriter.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, compressionLevel)
	})

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录本身
		if info.IsDir() {
			return nil
		}

		// 创建文件在 ZIP 中的路径
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		zipFileWriter, err := zipWriter.Create(relPath)

		//zipFileWriter, err := zipWriter.CreateHeader(&zip.FileHeader{
		//	Name:   relPath,
		//	Method: zip.Deflate,
		//})
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(zipFileWriter, file)
		return err
	})

	return err
}

// 调用 7-Zip 创建 ZIP 文件
func createZipWith7Zip(sourceDir, zipFileName string) error {
	cmd := exec.Command("7z", "a", "-tzip", "-mx=9", zipFileName, sourceDir)

	// 捕获命令输出
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("7z command failed: %w", err)
	}
	return nil
}

// 测试 ZIP 文件的完整性
func testZipIntegrity(zipFileName string) error {
	zipFile, err := os.Open(zipFileName)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	stat, err := zipFile.Stat()
	if err != nil {
		return err
	}

	// 创建 zip.Reader 读取 ZIP 文件
	zipReader, err := zip.NewReader(zipFile, stat.Size())
	if err != nil {
		return fmt.Errorf("error reading zip file: %w", err)
	}

	// 遍历 ZIP 文件中的每个条目
	for _, file := range zipReader.File {
		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("error testing zip file entry %s: %w", file.Name, err)
		}
		// 确保文件内容能被正常读取
		_, err = io.Copy(io.Discard, rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("error reading file %s in zip: %w", file.Name, err)
		}
	}
	return nil
}

// 调用 7-Zip 测试 ZIP 文件完整性
func testZipWith7Zip(zipFileName string) error {
	cmd := exec.Command("7z", "t", zipFileName)

	// 捕获命令输出
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("7z test command failed: %w", err)
	}
	return nil
}
