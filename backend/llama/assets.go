package llama

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kncept/quesadilla/utils/compress"
	"github.com/kncept/quesadilla/utils/qenv"
	"github.com/kncept/quesadilla/utils/qhttp"
)

// llamaAcceleration is the GPU backend selected for a llama.cpp install.
type llamaAcceleration int

const (
	llamaAccelerationCPU llamaAcceleration = iota
	llamaAccelerationCUDA
	llamaAccelerationVulkan
)

func (this llamaAcceleration) String() string {
	switch this {
	case llamaAccelerationCUDA:
		return "CUDA"
	case llamaAccelerationVulkan:
		return "Vulkan"
	default:
		return "CPU"
	}
}

// llamaCapabilities is what the current machine can accelerate llama.cpp with.
type llamaCapabilities struct {
	cudaVersion string
	vulkan      bool
}

// llamaAssetChoice is the single release asset selected for installation.
type llamaAssetChoice struct {
	name         string
	platform     string
	acceleration llamaAcceleration
	cudaVersion  string
	cudartName   string
}

func (this llamaAssetChoice) label() string {
	if this.acceleration == llamaAccelerationCUDA {
		return fmt.Sprintf("CUDA %s", this.cudaVersion)
	}
	return this.acceleration.String()
}

// installLlamaAssets is the binary runner's asset handler for the llama.cpp
// backend: it selects the release assets for the current platform, downloads
// and unpacks them, and tidies up the result.
func installLlamaAssets(version string, nameToDownloadUrl map[string]string) error {
	capabilities := detectLlamaCapabilities()
	choice, err := selectLlamaAsset(version, runtime.GOOS, runtime.GOARCH, nameToDownloadUrl, capabilities)
	if err != nil {
		return err
	}

	fmt.Printf("Downloading llama.cpp version %s (%s, %s)\n", version, choice.label(), choice.platform)

	downloadsDirectory := qenv.QBinariesDirectory(providerId)
	browserDownloadUrl := nameToDownloadUrl[choice.name]

	downloadDirectory := path.Join(downloadsDirectory, version)
	err = qhttp.DownloadFile(browserDownloadUrl, downloadDirectory, choice.name)
	if err != nil {
		return err
	}

	if strings.HasSuffix(choice.name, ".tar") {
		err = compress.Untar(path.Join(downloadDirectory, choice.name), downloadDirectory)
		if err != nil {
			return err
		}
	} else if strings.HasSuffix(choice.name, ".tar.gz") || strings.HasSuffix(choice.name, ".tgz") {
		err = compress.UntarGz(path.Join(downloadDirectory, choice.name), downloadDirectory)
		if err != nil {
			return err
		}
	} else if strings.HasSuffix(choice.name, ".zip") {
		err = compress.Unzip(path.Join(downloadDirectory, choice.name), downloadDirectory)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("unknown file format to handle: %s", choice.name)
	}
	err = os.Remove(path.Join(downloadDirectory, choice.name))
	if err != nil {
		return err
	}

	binDir := path.Join(downloadDirectory, fmt.Sprintf("llama-%s", version))
	if choice.cudartName != "" {
		fmt.Printf("Downloading CUDA runtime for llama.cpp version %s (%s, %s)\n", version, choice.label(), choice.platform)

		cudartPath := path.Join(downloadDirectory, choice.cudartName)
		err = qhttp.DownloadFile(nameToDownloadUrl[choice.cudartName], downloadDirectory, choice.cudartName)
		if err != nil {
			return err
		}
		err = installCudartLibraries(cudartPath, downloadDirectory, binDir)
		if err != nil {
			return err
		}
		err = os.Remove(cudartPath)
		if err != nil {
			return err
		}
	}

	// do linking here?

	dirEntries, err := os.ReadDir(binDir)
	if err != nil {
		return err
	}

	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		for _, dirEntry := range dirEntries {
			name := dirEntry.Name()
			if shouldSymlinkFile(name) {
				newname := newFileName(name)
				if dirEntry.Name() != newname {
					err = os.Symlink(name, path.Join(binDir, newname))
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// installCudartLibraries extracts the CUDA runtime archive and moves its
// libraries alongside libggml-cuda.so, where llama.cpp's backend loader can
// find them.
func installCudartLibraries(archivePath string, workDirectory string, binDir string) error {
	tempDirectory, err := os.MkdirTemp(workDirectory, "cudart-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDirectory)

	switch {
	case strings.HasSuffix(archivePath, ".tar"):
		err = compress.Untar(archivePath, tempDirectory)
	case strings.HasSuffix(archivePath, ".tar.gz"), strings.HasSuffix(archivePath, ".tgz"):
		err = compress.UntarGz(archivePath, tempDirectory)
	case strings.HasSuffix(archivePath, ".zip"):
		err = compress.Unzip(archivePath, tempDirectory)
	default:
		return fmt.Errorf("unknown file format to handle: %s", archivePath)
	}
	if err != nil {
		return err
	}

	return filepath.WalkDir(tempDirectory, func(currentPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}

		return os.Rename(currentPath, filepath.Join(binDir, entry.Name()))
	})
}

// detectLlamaCapabilities checks what GPU acceleration the current machine can
// use. Only Linux installs have separate CUDA/Vulkan assets; macOS releases
// bundle everything into one build.
func detectLlamaCapabilities() llamaCapabilities {
	if runtime.GOOS != "linux" {
		return llamaCapabilities{}
	}
	return llamaCapabilities{
		cudaVersion: detectNvidiaCudaVersion(),
		vulkan:      detectVulkanSupport(),
	}
}

const gpuDetectionTimeout = 10 * time.Second

// detectNvidiaCudaVersion runs nvidia-smi and extracts the CUDA version it
// reports. An empty result means there is no usable NVIDIA driver.
func detectNvidiaCudaVersion() string {
	output, _ := runGpuDetectionCommand("nvidia-smi")
	return parseNvidiaSmiCudaVersion(string(output))
}

var cudaVersionPattern = regexp.MustCompile(`CUDA Version:\s*([0-9]+(?:\.[0-9]+)*)`)

func parseNvidiaSmiCudaVersion(output string) string {
	match := cudaVersionPattern.FindStringSubmatch(output)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

// detectVulkanSupport asks vulkaninfo for a device summary. If vulkaninfo is
// not installed (or no Vulkan device is present), this returns false.
func detectVulkanSupport() bool {
	output, _ := runGpuDetectionCommand("vulkaninfo", "--summary")
	return parseVulkanSummary(string(output))
}

var vulkanDevicePattern = regexp.MustCompile(`(?m)^\s*(?:GPU[0-9]+:|deviceName\s*=)`)

func parseVulkanSummary(output string) bool {
	return vulkanDevicePattern.MatchString(output)
}

func runGpuDetectionCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gpuDetectionTimeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// selectLlamaAsset chooses a release asset for the platform, preferring CUDA
// and then Vulkan on Linux when those capabilities are available. It falls
// back to the CPU build when no accelerated build can be used.
func selectLlamaAsset(version string, goos string, goarch string, nameToDownloadUrl map[string]string, capabilities llamaCapabilities) (llamaAssetChoice, error) {
	osPattern, archPattern, err := llamaAssetPatterns(goos, goarch)
	if err != nil {
		return llamaAssetChoice{}, err
	}
	platform := fmt.Sprintf("%s-%s", osPattern, archPattern)

	if goos == "linux" {
		if capabilities.cudaVersion != "" {
			if name, cudaVersion, ok := selectCudaAsset(version, archPattern, nameToDownloadUrl, capabilities.cudaVersion); ok {
				cudartName := "cudart-" + name
				if _, ok := nameToDownloadUrl[cudartName]; !ok {
					// Some releases rely on the host's CUDA runtime instead of
					// publishing a companion archive.
					cudartName = ""
				}
				return llamaAssetChoice{
					name:         name,
					platform:     platform,
					acceleration: llamaAccelerationCUDA,
					cudaVersion:  cudaVersion,
					cudartName:   cudartName,
				}, nil
			}
		}
		if capabilities.vulkan {
			if name, ok := findLlamaVariantAsset(version, osPattern, archPattern, "vulkan", nameToDownloadUrl); ok {
				return llamaAssetChoice{
					name:         name,
					platform:     platform,
					acceleration: llamaAccelerationVulkan,
				}, nil
			}
		}
	}

	if name, ok := findCpuAsset(version, osPattern, archPattern, nameToDownloadUrl); ok {
		return llamaAssetChoice{
			name:         name,
			platform:     platform,
			acceleration: llamaAccelerationCPU,
		}, nil
	}

	return llamaAssetChoice{}, fmt.Errorf("unable to work out what llama.cpp asset to download for %s/%s (version %s)", goos, goarch, version)
}

// llamaAssetPatterns maps Go's OS/arch names to llama.cpp's release asset
// naming:
// - Linux: ubuntu-x64, ubuntu-arm64 (not linux-amd64)
// - macOS: macos-x64, macos-arm64
// - Windows: win-cpu-x64, win-cpu-arm64
func llamaAssetPatterns(goos string, goarch string) (osPattern string, archPattern string, err error) {
	switch goarch {
	case "amd64":
		archPattern = "x64"
	case "arm64":
		archPattern = "arm64"
	default:
		return "", "", fmt.Errorf("unsupported architecture: %s", goarch)
	}

	switch goos {
	case "linux":
		osPattern = "ubuntu"
	case "darwin":
		osPattern = "macos"
	case "windows":
		osPattern = "win-cpu"
	default:
		return "", "", fmt.Errorf("unsupported operating system: %s", goos)
	}

	return osPattern, archPattern, nil
}

// findCpuAsset finds the plain CPU build for the platform.
func findCpuAsset(version string, osPattern string, archPattern string, nameToDownloadUrl map[string]string) (string, bool) {
	assetPrefix := fmt.Sprintf("llama-%s-bin-%s-%s", version, osPattern, archPattern)
	for _, extension := range []string{".tar.gz", ".tgz", ".tar", ".zip"} {
		if _, ok := nameToDownloadUrl[assetPrefix+extension]; ok {
			return assetPrefix + extension, true
		}
	}

	// Fallback: match any CPU-looking asset for the os/arch pattern.
	for _, name := range sortedAssetNames(nameToDownloadUrl) {
		if strings.Contains(name, osPattern) && strings.Contains(name, archPattern) && !isAcceleratedLlamaAsset(name) {
			return name, true
		}
	}

	return "", false
}

// findLlamaVariantAsset finds a named accelerated build, e.g. the Vulkan asset
// llama-b12387-bin-ubuntu-vulkan-x64.tar.gz.
func findLlamaVariantAsset(version string, osPattern string, archPattern string, variant string, nameToDownloadUrl map[string]string) (string, bool) {
	assetPrefix := fmt.Sprintf("llama-%s-bin-%s-%s-", version, osPattern, variant)
	assetSuffix := fmt.Sprintf("-%s.tar.gz", archPattern)

	for _, name := range sortedAssetNames(nameToDownloadUrl) {
		if strings.HasPrefix(name, assetPrefix) && strings.HasSuffix(name, assetSuffix) {
			return name, true
		}
	}
	return "", false
}

// selectCudaAsset picks the best CUDA build available for the driver's CUDA
// version. Newer minor versions within the same CUDA major version are allowed
// by CUDA's minor version compatibility rules.
func selectCudaAsset(version string, archPattern string, nameToDownloadUrl map[string]string, driverVersion string) (string, string, bool) {
	driverNumbers := parseVersionNumbers(driverVersion)
	if len(driverNumbers) == 0 {
		return "", "", false
	}

	assetPrefix := fmt.Sprintf("llama-%s-bin-ubuntu-cuda-", version)
	assetSuffix := fmt.Sprintf("-%s.tar.gz", archPattern)

	bestName := ""
	bestNumbers := []int(nil)
	exactName := ""
	exactNumbers := []int(nil)
	for _, name := range sortedAssetNames(nameToDownloadUrl) {
		if !strings.HasPrefix(name, assetPrefix) || !strings.HasSuffix(name, assetSuffix) {
			continue
		}

		buildSegment := strings.TrimSuffix(strings.TrimPrefix(name, assetPrefix), assetSuffix)
		buildNumbers := parseVersionNumbers(buildSegment)
		if len(buildNumbers) == 0 || buildNumbers[0] > driverNumbers[0] {
			continue
		}

		if compareVersionNumbers(buildNumbers, driverNumbers) == 0 {
			exactName = name
			exactNumbers = buildNumbers
			continue
		}

		if bestName == "" || compareVersionNumbers(buildNumbers, bestNumbers) > 0 {
			bestName = name
			bestNumbers = buildNumbers
		}
	}

	if exactName != "" {
		return exactName, formatVersionNumbers(exactNumbers), true
	}
	if bestName == "" {
		return "", "", false
	}
	return bestName, formatVersionNumbers(bestNumbers), true
}

var versionNumbersPattern = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)*`)

func parseVersionNumbers(version string) []int {
	match := versionNumbersPattern.FindString(version)
	if match == "" {
		return nil
	}

	parts := strings.Split(match, ".")
	numbers := make([]int, 0, len(parts))
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		numbers = append(numbers, number)
	}
	return numbers
}

func compareVersionNumbers(a []int, b []int) int {
	length := len(a)
	if len(b) > length {
		length = len(b)
	}

	for i := 0; i < length; i++ {
		var aPart, bPart int
		if i < len(a) {
			aPart = a[i]
		}
		if i < len(b) {
			bPart = b[i]
		}
		if aPart > bPart {
			return 1
		}
		if aPart < bPart {
			return -1
		}
	}
	return 0
}

func formatVersionNumbers(numbers []int) string {
	parts := make([]string, len(numbers))
	for i, number := range numbers {
		parts[i] = strconv.Itoa(number)
	}
	return strings.Join(parts, ".")
}

func sortedAssetNames(nameToDownloadUrl map[string]string) []string {
	names := make([]string, 0, len(nameToDownloadUrl))
	for name := range nameToDownloadUrl {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func isAcceleratedLlamaAsset(name string) bool {
	for _, marker := range []string{
		"cuda", "rocm", "vulkan", "openvino", "sycl", "openblas",
		"opencl", "hip", "kompute", "llvm", "msvc", "zdnn", "snapdragon",
	} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// shouldSymlinkFile reports whether the extracted library needs a symlink
// alongside it so llama-server can locate it.
// On macOS: creates libllama.0.dylib -> libllama.0.0.0.dylib
// On Linux: creates libllama.so.0 -> libllama.so.0.0.0
func shouldSymlinkFile(filename string) bool {
	return strings.HasSuffix(filename, ".dylib") || strings.Contains(filename, ".so.")
}

// newFileName strips the full version from a dylib name, keeping only the
// major version (the version the dynamic loader looks for).
func newFileName(filename string) string {
	versionRenameRegex := regexp.MustCompile(`\.(\d+)\.\d+\.\d+`)
	return versionRenameRegex.ReplaceAllString(filename, ".$1")
}
