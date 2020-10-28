package sdk

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	lbplogger "github.com/buildpack/libbuildpack/logger"
	"github.com/cloudfoundry/libcfbuildpack/buildpackplan"
	"github.com/cloudfoundry/libcfbuildpack/layers"
	"github.com/cloudfoundry/libcfbuildpack/logger"
	"github.com/cloudfoundry/libcfbuildpack/test"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestUnitSdk(t *testing.T) {
	spec.Run(t, "Detect", testSdk, spec.Report(report.Terminal{}))
}

func testSdk(t *testing.T, when spec.G, it spec.S) {
	var (
		factory                 *test.BuildFactory
		stubDotnetSDKFixture    = filepath.Join("testdata", "stub-sdk-dependency.tar.xz")
		fakeSymlinkTarget       string
		runtimeSymlinkLayerPath string
		symlinkLayer            layers.Layer
		appRoot                 string
	)

	it.Before(func() {
		var err error

		RegisterTestingT(t)
		factory = test.NewBuildFactory(t)
		factory.AddDependencyWithVersion(DotnetSDK, "2.2.800", stubDotnetSDKFixture)

		runtimetoSdks := make([]map[string]interface{}, 0)

		runtimetoSdks = append(runtimetoSdks, map[string]interface{}{
			"runtime-version": "2.1.13",
			"sdks":            []string{"2.2.800", "2.2.805"},
		})

		runtimetoSdks = append(runtimetoSdks, map[string]interface{}{
			"runtime-version": "2.1.12",
			"sdks":            []string{"2.2.700"},
		})
		factory.Build.Buildpack.Metadata["runtime-to-sdks"] = runtimetoSdks

		symlinkLayer = factory.Build.Layers.Layer("driver-symlinks")

		fakeSymlinkTarget, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		runtimeSymlinkLayerPath, err = ioutil.TempDir(os.TempDir(), "runtime")
		Expect(err).ToNot(HaveOccurred())

		Expect(os.MkdirAll(filepath.Join(runtimeSymlinkLayerPath, "shared"), os.ModePerm)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(runtimeSymlinkLayerPath, "host"), os.ModePerm)).To(Succeed())
		Expect(os.Symlink(fakeSymlinkTarget, filepath.Join(runtimeSymlinkLayerPath, "shared", "Microsoft.NETCore.App"))).To(Succeed())
		Expect(os.Symlink(fakeSymlinkTarget, filepath.Join(runtimeSymlinkLayerPath, "host", "fxr"))).To(Succeed())

		appRoot = factory.Build.Application.Root

		os.Setenv("DOTNET_ROOT", runtimeSymlinkLayerPath)
	})

	it.After(func() {
		os.RemoveAll(fakeSymlinkTarget)
		os.RemoveAll(runtimeSymlinkLayerPath)
		os.Unsetenv("DOTNET_ROOT")
	})

	when("runtime.NewContributor", func() {
		when("RUNTIME_VERSION is 2.1.13", func() {

			it.Before(func() {
				os.Setenv("RUNTIME_VERSION", "2.1.13")
			})
			it.After(func() {
				os.Unsetenv("RUNTIME_VERSION")
			})

			when("there is not buildpack.yml or global.json", func() {
				it("returns true if a build plan exists and it finds a compatible sdk version", func() {
					factory.AddPlan(buildpackplan.Plan{Name: DotnetSDK, Version: "2.2.0"})
					factory.AddPlan(buildpackplan.Plan{Name: DotnetSDK, Version: "2.2.0"})

					contributor, willContribute, err := NewContributor(factory.Build)
					Expect(err).NotTo(HaveOccurred())
					Expect(willContribute).To(BeTrue())
					Expect(contributor.sdkLayer.Dependency.Version.String()).To(Equal("2.2.800"))
				})
			})

			when("there is a buildpack.yml", func() {
				it.Before(func() {
					factory.AddDependencyWithVersion(DotnetSDK, "2.2.805", stubDotnetSDKFixture)
					factory.AddPlan(buildpackplan.Plan{Name: DotnetSDK, Version: "2.2.0"})
				})

				it.After(func() {
					os.RemoveAll(filepath.Join(appRoot, "buildpack.yml"))
				})

				it("use specfic version in buildpack.yml", func() {
					Expect(ioutil.WriteFile(filepath.Join(appRoot, "buildpack.yml"), []byte(`---
dotnet-sdk:
  version: "2.2.800"
`), 0644)).To(Succeed())

					contributor, willContribute, err := NewContributor(factory.Build)
					Expect(err).NotTo(HaveOccurred())
					Expect(willContribute).To(BeTrue())
					Expect(contributor.sdkLayer.Dependency.Version.String()).To(Equal("2.2.800"))
				})

				it("use feature line constraint in buildpack.yml", func() {
					Expect(ioutil.WriteFile(filepath.Join(appRoot, "buildpack.yml"), []byte(`---
dotnet-sdk:
  version: "2.2.8*"
`), 0644)).To(Succeed())

					contributor, willContribute, err := NewContributor(factory.Build)
					Expect(err).NotTo(HaveOccurred())
					Expect(willContribute).To(BeTrue())
					Expect(contributor.sdkLayer.Dependency.Version.String()).To(Equal("2.2.805"))
				})

				it("use minor constraint in buildpack.yml", func() {
					Expect(ioutil.WriteFile(filepath.Join(appRoot, "buildpack.yml"), []byte(`---
dotnet-sdk:
  version: "2.2.*"
`), 0644)).To(Succeed())

					contributor, willContribute, err := NewContributor(factory.Build)
					Expect(err).NotTo(HaveOccurred())
					Expect(willContribute).To(BeTrue())
					Expect(contributor.sdkLayer.Dependency.Version.String()).To(Equal("2.2.805"))
				})
			})

			when("there is a global.json", func() {
				it.Before(func() {
					factory.AddDependencyWithVersion(DotnetSDK, "2.2.805", stubDotnetSDKFixture)
					factory.AddPlan(buildpackplan.Plan{Name: DotnetSDK, Version: "2.2.0"})
				})

				it.After(func() {
					os.RemoveAll(filepath.Join(appRoot, "global.json"))
				})

				when("exact version exist in global.json and buildpack.toml", func() {
					it("use the exact version", func() {
						Expect(ioutil.WriteFile(filepath.Join(appRoot, "global.json"), []byte(`{
"sdk": {"version" : "2.2.800"}
}
`), 0644)).To(Succeed())

						contributor, willContribute, err := NewContributor(factory.Build)
						Expect(err).NotTo(HaveOccurred())
						Expect(willContribute).To(BeTrue())
						Expect(contributor.sdkLayer.Dependency.Version.String()).To(Equal("2.2.800"))
					})
				})

				when("feature line compatible version exist in global.json and buildpack.toml", func() {
					it("use the feature line compatible version", func() {
						Expect(ioutil.WriteFile(filepath.Join(appRoot, "global.json"), []byte(`{
"sdk": {"version" : "2.2.801"}
}
`), 0644)).To(Succeed())

						contributor, willContribute, err := NewContributor(factory.Build)
						Expect(err).NotTo(HaveOccurred())
						Expect(willContribute).To(BeTrue())
						Expect(contributor.sdkLayer.Dependency.Version.String()).To(Equal("2.2.805"))
					})
				})
			})

			when("there is both a buildpack.yml and a global.json", func() {
				it.Before(func() {
					factory.AddDependencyWithVersion(DotnetSDK, "2.2.805", stubDotnetSDKFixture)
					factory.AddPlan(buildpackplan.Plan{Name: DotnetSDK, Version: "2.2.0"})
				})

				it.After(func() {
					os.RemoveAll(filepath.Join(appRoot, "buildpack.yml"))
					os.RemoveAll(filepath.Join(appRoot, "global.json"))
				})

				when("feature line compatible version exist in global.json and buildpack.yml", func() {
					it("uses buildpack.yml", func() {
						Expect(ioutil.WriteFile(filepath.Join(appRoot, "global.json"), []byte(`{
"sdk": {"version" : "2.2.800"}
}
`), 0644)).To(Succeed())
						Expect(ioutil.WriteFile(filepath.Join(appRoot, "buildpack.yml"), []byte(`---
dotnet-sdk:
 version: "2.2.805"
`), 0644)).To(Succeed())

						contributor, willContribute, err := NewContributor(factory.Build)
						Expect(err).NotTo(HaveOccurred())
						Expect(willContribute).To(BeTrue())
						Expect(contributor.sdkLayer.Dependency.Version.String()).To(Equal("2.2.805"))
					})
				})
			})

			it("returns false if a build plan exists and it does not find a compatible sdk version", func() {
				factory.AddPlan(buildpackplan.Plan{Name: DotnetSDK, Version: "2.1.0"})

				_, willContribute, err := NewContributor(factory.Build)
				Expect(err).To(HaveOccurred())
				Expect(willContribute).To(BeFalse())
			})

			it("returns false if a build plan does not exist", func() {
				_, willContribute, err := NewContributor(factory.Build)
				Expect(err).NotTo(HaveOccurred())
				Expect(willContribute).To(BeFalse())
			})
		})
		when("RUNTIME_VERSION is 2.1.12", func() {
			it.Before(func() {
				os.Setenv("RUNTIME_VERSION", "2.1.12")
			})
			it.After(func() {
				os.Unsetenv("RUNTIME_VERSION")
			})

			when("there is not buildpack.yml or global.json", func() {
				it("returns true if a build plan exists and it finds a compatible sdk version", func() {
					factory.AddDependencyWithVersion(DotnetSDK, "2.2.700", stubDotnetSDKFixture)
					factory.AddPlan(buildpackplan.Plan{Name: DotnetSDK, Version: "2.2.0"})

					contributor, willContribute, err := NewContributor(factory.Build)
					Expect(err).NotTo(HaveOccurred())
					Expect(willContribute).To(BeTrue())
					Expect(contributor.sdkLayer.Dependency.Version.String()).To(Equal("2.2.700"))
				})
			})
		})
	})

	when("Contribute", func() {
		it("does not rebuild symlink layers when there is no SDK contribution", func() {

			outputBytes := bytes.Buffer{}
			debugBytes := bytes.Buffer{}
			sublogger := lbplogger.NewLogger(bufio.NewWriter(&debugBytes), bufio.NewWriter(&outputBytes))

			factory.AddPlan(buildpackplan.Plan{Name: DotnetSDK, Version: "2.2.0"})
			contributor1, willContribute, err := NewContributor(factory.Build)
			Expect(err).ToNot(HaveOccurred())
			Expect(willContribute).To(BeTrue())

			contributor1.sdkSymlinkLayer.Logger = logger.Logger{Logger: sublogger}

			Expect(contributor1.Contribute()).To(Succeed())
			stripedOutputFirst := StripANSIColor(outputBytes.String())
			Expect(stripedOutputFirst).To(ContainSubstring("Symlinking runtime libraries"))

			outputBytes.Reset()
			debugBytes.Reset()

			contributor2, willContribute, err := NewContributor(factory.Build)
			Expect(err).ToNot(HaveOccurred())
			Expect(willContribute).To(BeTrue())

			contributor2.sdkSymlinkLayer.Logger = logger.Logger{Logger: sublogger}
			Expect(contributor2.Contribute()).To(Succeed())

			stripedOutputSecond := StripANSIColor(outputBytes.String())
			Expect(stripedOutputSecond).ToNot(ContainSubstring("Symlinking runtime libraries"))

		})

		it("appends dotnet driver to path, installs the runtime dependency", func() {
			factory.AddPlan(buildpackplan.Plan{Name: DotnetSDK, Version: "2.2.0"})

			dotnetSDKContributor, _, err := NewContributor(factory.Build)
			Expect(err).NotTo(HaveOccurred())

			Expect(dotnetSDKContributor.Contribute()).To(Succeed())

			Expect(dotnetSDKContributor.sdkLayer).To(test.HaveOverrideBuildEnvironment("SDK_LOCATION", dotnetSDKContributor.sdkLayer.Root))

			ExpectSymlink(filepath.Join(symlinkLayer.Root, "host"), t)

			Expect(filepath.Join(symlinkLayer.Root, "shared")).To(BeADirectory())

			ExpectSymlink(filepath.Join(symlinkLayer.Root, "shared", "Microsoft.NETCore.App"), t)

			Expect(symlinkLayer).To(test.HavePrependPathSharedEnvironment("PATH", filepath.Join(symlinkLayer.Root)))
			Expect(symlinkLayer).To(test.HaveOverrideSharedEnvironment("DOTNET_ROOT", filepath.Join(symlinkLayer.Root)))
		})

		it("contributes dotnet runtime to the build layer when included in the build plan", func() {
			factory.AddPlan(buildpackplan.Plan{
				Name:    DotnetSDK,
				Version: "2.2.0",
				Metadata: buildpackplan.Metadata{
					"build": true,
				},
			})

			dotnetSDKContributor, _, err := NewContributor(factory.Build)
			Expect(err).NotTo(HaveOccurred())

			Expect(dotnetSDKContributor.Contribute()).To(Succeed())

			layer := factory.Build.Layers.Layer(DotnetSDK)
			Expect(layer).To(test.HaveLayerMetadata(true, false, false))
			Expect(symlinkLayer).To(test.HaveLayerMetadata(true, false, false))
		})

		it("contributes dotnet runtime to the launch layer when included in the build plan", func() {
			factory.AddPlan(buildpackplan.Plan{
				Name:    DotnetSDK,
				Version: "2.2.0",
				Metadata: buildpackplan.Metadata{
					"launch": true,
				},
			})

			dotnetSDKContributor, _, err := NewContributor(factory.Build)
			Expect(err).NotTo(HaveOccurred())

			Expect(dotnetSDKContributor.Contribute()).To(Succeed())

			layer := factory.Build.Layers.Layer(DotnetSDK)
			Expect(layer).To(test.HaveLayerMetadata(true, false, false))
			Expect(symlinkLayer).To(test.HaveLayerMetadata(false, false, true))
		})
	})

	when("GetRuntimetoSDKMap", func() {
		it("correctly parses SDK map out of buildpack.toml", func() {
			mapping, err := GetRuntimetoSDKMap(factory.Build)
			Expect(err).NotTo(HaveOccurred())

			expectedMap := map[string][]string{"2.1.13": []string{"2.2.800", "2.2.805"}, "2.1.12": []string{"2.2.700"}}
			Expect(mapping).To(Equal(expectedMap))
		})
	})

}

func ExpectSymlink(path string, t *testing.T) {
	t.Helper()
	hostFileInfo, err := os.Stat(path)
	Expect(err).ToNot(HaveOccurred())
	Expect(hostFileInfo.Mode() & os.ModeSymlink).ToNot(Equal(0))
}

func StripANSIColor(str string) string {
	ansi := "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"
	re := regexp.MustCompile(ansi)
	return re.ReplaceAllString(str, "")
}
