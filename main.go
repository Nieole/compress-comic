package main

import (
	"archive/zip"
	"compress/flate"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// 进度信息结构
type Progress struct {
	totalChapters  int
	processedCount int
	failedCount    int
	mu             sync.Mutex
	startTime      time.Time
}

func (p *Progress) increment(success bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.processedCount++
	if !success {
		p.failedCount++
	}
}

func (p *Progress) print() {
	p.mu.Lock()
	defer p.mu.Unlock()
	elapsed := time.Since(p.startTime)
	fmt.Printf("\r处理进度: %d/%d 完成 (失败: %d) - 耗时: %v",
		p.processedCount, p.totalChapters, p.failedCount, elapsed.Round(time.Second))
}

// 检查目录是否只包含图片文件且没有子目录
func isImageOnlyDir(dirPath string) (bool, error) {
	hasImages := false
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			return false, nil // 有子目录，直接返回false
		}

		if !entry.Type().IsRegular() {
			continue // 跳过非普通文件
		}

		ext := strings.ToUpper(filepath.Ext(entry.Name()))

		// 如果目录中包含压缩包，跳过该目录
		if ext == ".ZIP" || ext == ".RAR" || ext == ".7Z" {
			return false, nil
		}

		if ext == ".JPG" || ext == ".JPEG" || ext == ".PNG" ||
			ext == ".WEBP" || ext == ".AVIF" || ext == ".GIF" ||
			ext == ".BMP" || ext == ".HEIC" || ext == ".HEIF" ||
			ext == ".TIFF" || ext == ".TIF" {
			hasImages = true
		}
	}

	return hasImages, nil
}

func main() {
	// 定义命令行参数
	var (
		rootDir     string
		workerCount int
		verbose     bool
	)

	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal("获取当前工作目录失败:", err)
	}

	defaultDir := filepath.Join(pwd, "comic")

	// 设置默认的工作线程数
	defaultWorkers := runtime.NumCPU()
	if defaultWorkers > 8 {
		defaultWorkers = 8
	}

	flag.StringVar(&rootDir, "dir", defaultDir, "漫画根目录路径 (支持相对路径)")
	flag.IntVar(&workerCount, "workers", defaultWorkers, "并发处理的工作线程数")
	flag.BoolVar(&verbose, "v", false, "显示详细的压缩过程信息")
	flag.Parse()

	// 显示版本信息
	if verbose {
		fmt.Printf("Go版本: %s\n", runtime.Version())
		fmt.Printf("操作系统/架构: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	}

	// 处理相对路径
	if !filepath.IsAbs(rootDir) {
		rootDir = filepath.Join(pwd, rootDir)
	}

	// 规范化路径
	rootDir = filepath.Clean(rootDir)

	// 检查目录是否存在
	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
		log.Printf("目录 %s 不存在", rootDir)
		return
	}

	fmt.Printf("处理目录: %s\n", rootDir)
	fmt.Printf("并发数量: %d\n", workerCount)

	// 收集所有符合条件的目录
	var chapterDirs []string

	// 使用更高效的目录遍历
	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Println(err)
			return filepath.SkipDir
		}

		if path == rootDir {
			return nil
		}

		if info.IsDir() {
			isImageDir, err := isImageOnlyDir(path)
			if err != nil {
				log.Printf("检查目录失败 %s: %v\n", path, err)
				return filepath.SkipDir
			}

			if isImageDir {
				chapterDirs = append(chapterDirs, path)
			}
		}
		return nil
	})

	if err != nil {
		fmt.Printf("遍历目录时发生错误: %v\n", err)
		return
	}

	if len(chapterDirs) == 0 {
		fmt.Println("未找到需要处理的章节目录")
		return
	}

	fmt.Printf("找到 %d 个待处理目录\n", len(chapterDirs))

	// 创建进度跟踪器
	progress := &Progress{
		totalChapters: len(chapterDirs),
		startTime:     time.Now(),
	}

	// 创建工作通道
	jobs := make(chan string, len(chapterDirs))
	var wg sync.WaitGroup

	// 启动工作协程
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for chapterDir := range jobs {
				err := processChapter(chapterDir, verbose)
				progress.increment(err == nil)
				progress.print()
			}
		}()
	}

	// 分发工作
	for _, chapterDir := range chapterDirs {
		jobs <- chapterDir
	}
	close(jobs)

	// 等待所有工作完成
	wg.Wait()
	fmt.Println("\n处理完成!")
}

// 处理单个章节文件夹
func processChapter(chapterDir string, verbose bool) error {
	zipFileName := chapterDir + ".zip"

	// 创建压缩包
	err := createZipWith7Zip(chapterDir, zipFileName, verbose)
	if err != nil {
		return fmt.Errorf("压缩失败 %s: %w", chapterDir, err)
	}

	// 验证压缩包完整性
	err = testZipWith7Zip(zipFileName, verbose)
	if err != nil {
		return fmt.Errorf("验证失败 %s: %w", chapterDir, err)
	}

	// 删除章节文件夹
	err = os.RemoveAll(chapterDir)
	if err != nil {
		return fmt.Errorf("删除原目录失败 %s: %w", chapterDir, err)
	}

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
func createZipWith7Zip(sourceDir, zipFileName string, verbose bool) error {
	cmd := exec.Command("7z", "a", "-tzip", "-mx=9", zipFileName, sourceDir)

	// 根据verbose标记决定是否显示输出
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

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
func testZipWith7Zip(zipFileName string, verbose bool) error {
	cmd := exec.Command("7z", "t", zipFileName)

	// 根据verbose标记决定是否显示输出
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("7z test command failed: %w", err)
	}
	return nil
}
