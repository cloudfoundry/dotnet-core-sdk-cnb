package dotnetcoresdk_test

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	dotnetcoresdk "github.com/paketo-buildpacks/dotnet-core-sdk"
	"github.com/paketo-buildpacks/dotnet-core-sdk/fakes"
	"github.com/paketo-buildpacks/packit"
	"github.com/paketo-buildpacks/packit/chronos"
	"github.com/paketo-buildpacks/packit/postal"
	"github.com/sclevine/spec"

	. "github.com/onsi/gomega"
)

func testBuild(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect = NewWithT(t).Expect

		layersDir  string
		cnbDir     string
		workingDir string
		clock      chronos.Clock
		timeStamp  time.Time
		buffer     *bytes.Buffer

		entryResolver      *fakes.EntryResolver
		dependencyResolver *fakes.DependencyResolver
		dependencyManager  *fakes.DependencyManager
		dotnetSymlinker    *fakes.DotnetSymlinker
		buildPlanRefinery  *fakes.BuildPlanRefinery

		build packit.BuildFunc
	)

	it.Before(func() {
		var err error

		layersDir, err = ioutil.TempDir("", "layers")
		Expect(err).NotTo(HaveOccurred())

		cnbDir, err = ioutil.TempDir("", "cnb")
		Expect(err).NotTo(HaveOccurred())

		workingDir, err = ioutil.TempDir("", "working-dir")
		Expect(err).NotTo(HaveOccurred())

		entryResolver = &fakes.EntryResolver{}
		entryResolver.ResolveCall.Returns.BuildpackPlanEntry = packit.BuildpackPlanEntry{
			Name: "dotnet-sdk",
			Metadata: map[string]interface{}{
				"version-source": "buildpack.yml",
				"version":        "2.5.x",
				"build":          true,
				"launch":         true,
			},
		}

		dependencyResolver = &fakes.DependencyResolver{}
		dependencyResolver.ResolveCall.Returns.Dependency = postal.Dependency{
			ID:      "dotnet-sdk",
			Version: "some-version",
			Name:    "Dotnet Core SDK",
			SHA256:  "some-sha",
		}

		buildPlanRefinery = &fakes.BuildPlanRefinery{}
		buildPlanRefinery.BillOfMaterialCall.Returns.BuildpackPlanEntry = packit.BuildpackPlanEntry{
			Name: "dotnet-sdk",
			Metadata: map[string]interface{}{
				"version":  "2.5.x",
				"licenses": []string{},
				"name":     "dotnet-sdk",
				"sha256":   "some-sha",
			},
		}

		dependencyManager = &fakes.DependencyManager{}
		dotnetSymlinker = &fakes.DotnetSymlinker{}

		buffer = bytes.NewBuffer(nil)
		logEmitter := dotnetcoresdk.NewLogEmitter(buffer)

		timeStamp = time.Now()
		clock = chronos.NewClock(func() time.Time {
			return timeStamp
		})

		build = dotnetcoresdk.Build(
			entryResolver,
			dependencyResolver,
			buildPlanRefinery,
			dependencyManager,
			dotnetSymlinker,
			logEmitter,
			clock,
		)
	})

	it.After(func() {
		Expect(os.RemoveAll(layersDir)).To(Succeed())
		Expect(os.RemoveAll(cnbDir)).To(Succeed())
		Expect(os.RemoveAll(workingDir)).To(Succeed())
	})

	it("returns a result that installs a version of the SDK into a layer", func() {
		result, err := build(packit.BuildContext{
			Plan: packit.BuildpackPlan{
				Entries: []packit.BuildpackPlanEntry{
					{
						Name: "dotnet-sdk",
						Metadata: map[string]interface{}{
							"version-source": "buildpack.yml",
							"version":        "2.5.x",
							"build":          true,
							"launch":         true,
						},
					},
				},
			},
			Layers:     packit.Layers{Path: layersDir},
			CNBPath:    cnbDir,
			WorkingDir: workingDir,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(packit.BuildResult{
			Plan: packit.BuildpackPlan{
				Entries: []packit.BuildpackPlanEntry{
					{
						Name: "dotnet-sdk",
						Metadata: map[string]interface{}{
							"version":  "2.5.x",
							"licenses": []string{},
							"name":     "dotnet-sdk",
							"sha256":   "some-sha",
						},
					},
				},
			},
			Layers: []packit.Layer{
				{
					Name: "dotnet-core-sdk",
					Path: filepath.Join(layersDir, "dotnet-core-sdk"),
					SharedEnv: packit.Environment{
						"PATH.prepend":         filepath.Join(workingDir, ".dotnet_root"),
						"PATH.delim":           string(os.PathListSeparator),
						"DOTNET_ROOT.override": filepath.Join(workingDir, ".dotnet_root"),
					},
					BuildEnv:  packit.Environment{},
					LaunchEnv: packit.Environment{},
					Build:     true,
					Launch:    true,
					Cache:     true,
					Metadata: map[string]interface{}{
						"dependency-sha": "some-sha",
						"built_at":       timeStamp.Format(time.RFC3339Nano),
					},
				},
			},
		}))

		Expect(entryResolver.ResolveCall.Receives.Entries).
			To(Equal([]packit.BuildpackPlanEntry{
				{
					Name: "dotnet-sdk",
					Metadata: map[string]interface{}{
						"version-source": "buildpack.yml",
						"version":        "2.5.x",
						"build":          true,
						"launch":         true,
					},
				},
			}))

		Expect(dependencyResolver.ResolveCall.Receives.Entry).
			To(Equal(packit.BuildpackPlanEntry{
				Name: "dotnet-sdk",
				Metadata: map[string]interface{}{
					"version-source": "buildpack.yml",
					"version":        "2.5.x",
					"build":          true,
					"launch":         true,
				},
			}))

		Expect(buildPlanRefinery.BillOfMaterialCall.Receives.Dependency).
			To(Equal(postal.Dependency{
				ID:      "dotnet-sdk",
				Version: "some-version",
				Name:    "Dotnet Core SDK",
				SHA256:  "some-sha",
			}))

		Expect(dependencyManager.InstallCall.Receives.Dependency).
			To(Equal(postal.Dependency{
				ID:      "dotnet-sdk",
				Name:    "Dotnet Core SDK",
				Version: "some-version",
				SHA256:  "some-sha",
			}))
		Expect(dependencyManager.InstallCall.Receives.CnbPath).To(Equal(cnbDir))
		Expect(dependencyManager.InstallCall.Receives.LayerPath).To(Equal(filepath.Join(layersDir, "dotnet-core-sdk")))

		Expect(dotnetSymlinker.LinkCall.Receives.WorkingDir).To(Equal(workingDir))
		Expect(dotnetSymlinker.LinkCall.Receives.LayerPath).To(Equal(filepath.Join(layersDir, "dotnet-core-sdk")))
	})

	context("when there is a dependency cache match", func() {
		it.Before(func() {
			err := ioutil.WriteFile(filepath.Join(layersDir, "dotnet-core-sdk.toml"),
				[]byte("[metadata]\ndependency-sha = \"some-sha\"\n"), 0600)
			Expect(err).NotTo(HaveOccurred())
		})

		it("reuses the cached version of the SDK dependency", func() {
			_, err := build(packit.BuildContext{
				Plan: packit.BuildpackPlan{
					Entries: []packit.BuildpackPlanEntry{
						{
							Name: "dotnet-sdk",
							Metadata: map[string]interface{}{
								"version-source": "buildpack.yml",
								"version":        "2.5.x",
								"build":          true,
								"launch":         true,
							},
						},
					},
				},
				Layers:     packit.Layers{Path: layersDir},
				CNBPath:    cnbDir,
				WorkingDir: workingDir,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(entryResolver.ResolveCall.Receives.Entries).
				To(Equal([]packit.BuildpackPlanEntry{
					{
						Name: "dotnet-sdk",
						Metadata: map[string]interface{}{
							"version-source": "buildpack.yml",
							"version":        "2.5.x",
							"build":          true,
							"launch":         true,
						},
					},
				}))

			Expect(dependencyResolver.ResolveCall.Receives.Entry).
				To(Equal(packit.BuildpackPlanEntry{
					Name: "dotnet-sdk",
					Metadata: map[string]interface{}{
						"version-source": "buildpack.yml",
						"version":        "2.5.x",
						"build":          true,
						"launch":         true,
					},
				}))

			Expect(buildPlanRefinery.BillOfMaterialCall.Receives.Dependency).
				To(Equal(postal.Dependency{
					ID:      "dotnet-sdk",
					Version: "some-version",
					Name:    "Dotnet Core SDK",
					SHA256:  "some-sha",
				}))

			Expect(dependencyManager.InstallCall.CallCount).To(Equal(0))

			Expect(dotnetSymlinker.LinkCall.CallCount).To(Equal(1))
			Expect(dotnetSymlinker.LinkCall.Receives.WorkingDir).To(Equal(workingDir))
			Expect(dotnetSymlinker.LinkCall.Receives.LayerPath).To(Equal(filepath.Join(layersDir, "dotnet-core-sdk")))
		})
	})
	context("failure cases", func() {
		context("when the dependency for the build plan entry cannot be resolved", func() {
			it.Before(func() {
				dependencyResolver.ResolveCall.Returns.Error = errors.New("some-resolution-error")
			})
			it("returns an error", func() {
				_, err := build(packit.BuildContext{
					Plan: packit.BuildpackPlan{
						Entries: []packit.BuildpackPlanEntry{
							{
								Name: "dotnet-sdk",
								Metadata: map[string]interface{}{
									"version-source": "buildpack.yml",
									"version":        "2.5.x",
									"build":          true,
									"launch":         true,
								},
							},
						},
					},
					Layers:     packit.Layers{Path: layersDir},
					CNBPath:    cnbDir,
					WorkingDir: workingDir,
				})

				Expect(err).To(MatchError("some-resolution-error"))
			})
		})

		context("when layer dir cannot be accessed", func() {
			it.Before(func() {
				Expect(os.Chmod(layersDir, 0000)).To(Succeed())
			})

			it.After(func() {
				Expect(os.Chmod(layersDir, 0600)).To(Succeed())
			})

			it("returns an error", func() {
				_, err := build(packit.BuildContext{
					Plan: packit.BuildpackPlan{
						Entries: []packit.BuildpackPlanEntry{
							{
								Name: "dotnet-sdk",
								Metadata: map[string]interface{}{
									"version-source": "buildpack.yml",
									"version":        "2.5.x",
									"build":          true,
									"launch":         true,
								},
							},
						},
					},
					Layers:     packit.Layers{Path: layersDir},
					CNBPath:    cnbDir,
					WorkingDir: workingDir,
				})

				Expect(err).To(MatchError(ContainSubstring("permission denied")))
			})
		})

		context("when the dependency for the build plan entry cannot be resolved", func() {
			it.Before(func() {
				dependencyManager.InstallCall.Returns.Error = errors.New("some-installation-error")
			})
			it("returns an error", func() {
				_, err := build(packit.BuildContext{
					Plan: packit.BuildpackPlan{
						Entries: []packit.BuildpackPlanEntry{
							{
								Name: "dotnet-sdk",
								Metadata: map[string]interface{}{
									"version-source": "buildpack.yml",
									"version":        "2.5.x",
									"build":          true,
									"launch":         true,
								},
							},
						},
					},
					Layers:     packit.Layers{Path: layersDir},
					CNBPath:    cnbDir,
					WorkingDir: workingDir,
				})

				Expect(err).To(MatchError("some-installation-error"))
			})
		})

		context("when symlinking fails", func() {
			it.Before(func() {
				dotnetSymlinker.LinkCall.Returns.Error = errors.New("some-symlinking-error")
			})
			it("returns an error", func() {
				_, err := build(packit.BuildContext{
					Plan: packit.BuildpackPlan{
						Entries: []packit.BuildpackPlanEntry{
							{
								Name: "dotnet-sdk",
								Metadata: map[string]interface{}{
									"version-source": "buildpack.yml",
									"version":        "2.5.x",
									"build":          true,
									"launch":         true,
								},
							},
						},
					},
					Layers:     packit.Layers{Path: layersDir},
					CNBPath:    cnbDir,
					WorkingDir: workingDir,
				})

				Expect(err).To(MatchError("some-symlinking-error"))
			})
		})
	})
}
