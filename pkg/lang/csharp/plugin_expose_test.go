package csharp

import (
	"sort"
	"strings"
	"testing"

	"github.com/klothoplatform/klotho/pkg/compiler/types"
	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/graph"
	"github.com/stretchr/testify/assert"
)

func Test_findIApplicationBuilder(t *testing.T) {
	type expectations struct {
		startupClass           string
		appBuilderIdentifier   string
		routeBuilderIdentifier string
	}
	tests := []struct {
		name         string
		program      string
		expectations []expectations
	}{
		{
			name: "Finds Annotated Startup Classes",
			program: `
			using Microsoft.AspNetconstruct.Builder;
			using Microsoft.AspNetconstruct.Hosting;
			
			public class MyStartupClass {
				public void Configure(IApplicationBuilder app, IWebHostEnvironment env)
				{
					/**
					* @klotho::expose {
					*  id = "csharp-gateway"
					*  target = "public"
					* }
					*/
					app.UseEndpoints(endpoints =>
					{				
						endpoints.MapGet("/}", () => "Hello!");
					});
				}
			}
			
			public class MyQualifiedStartupClass {
				public void Configure(
					Microsoft.AspNetconstruct.Builder.IApplicationBuilder qualifiedApp,
					Microsoft.AspNetconstruct.Hosting.IWebHostEnvironment env)
				{
					/**
					* @klotho::expose {
					*  id = "csharp-gateway2"
					*  target = "public"
					* }
					*/
					qualifiedApp.UseEndpoints(endpoints =>
					{				
						endpoints.MapGet("/}", () => "Hello!");
					});
				}
			}
			
			public class InvalidStartupClassWrongArgType {
				public void Configure(SomeOtherType app, IWebHostEnvironment env)
				{
					/**
					* @klotho::expose {
					*  id = "csharp-gateway3"
					*  target = "public"
					* }
					*/
					app.UseEndpoints(endpoints =>
					{				
						endpoints.MapGet("/}", () => "Hello!");
					});
				}
			}
			
			public class InvalidStartupClassNoConfigureMethod {
				public void OtherMethod(IApplicationBuilder app, IWebHostEnvironment env)
				{
					/**
					* @klotho::expose {
					*  id = "csharp-gateway4"
					*  target = "public"
					* }
					*/
					app.UseEndpoints(endpoints =>
					{				
						endpoints.MapGet("/}", () => "Hello!");
					});
				}
			}
			
			public class InvalidNonAnnotatedStartupClass {
				public void Configure(IApplicationBuilder app, IWebHostEnvironment env)
				{
					app.UseEndpoints(endpoints =>
					{				
						endpoints.MapGet("/}", () => "Hello!");
					});
				}
			}
			
				`,
			expectations: []expectations{
				{
					startupClass:           "MyStartupClass",
					appBuilderIdentifier:   "app",
					routeBuilderIdentifier: "endpoints",
				},
				{
					startupClass:           "MyQualifiedStartupClass",
					appBuilderIdentifier:   "qualifiedApp",
					routeBuilderIdentifier: "endpoints",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)
			file, err := types.NewSourceFile("program.cs", strings.NewReader(tt.program), Language)
			if !assert.NoError(err) {
				return
			}
			var actual []expectations
			for _, a := range file.Annotations() {
				if a.Capability.Name == "expose" {
					results := findIApplicationBuilder(a)
					for _, r := range results {
						actual = append(actual, expectations{
							startupClass:           r.StartupClass.Class.QualifiedName,
							appBuilderIdentifier:   r.AppBuilderIdentifier.Content(),
							routeBuilderIdentifier: r.EndpointRouteBuilderIdentifier.Content(),
						})
					}
				}
			}

			sort.Slice(tt.expectations, func(i, j int) bool {
				return tt.expectations[i].startupClass < tt.expectations[j].startupClass
			})
			sort.Slice(actual, func(i, j int) bool {
				return actual[i].startupClass < actual[j].startupClass
			})

			assert.Equal(len(tt.expectations), len(actual), "Incorrect number of results")
			assert.Equal(tt.expectations, actual)
		})
	}
}

func TestExpose_Transform(t *testing.T) {
	controllerMappingStartupClass := `
	using Microsoft.AspNetconstruct.Builder;
	using Microsoft.AspNetconstruct.Hosting;
	using Microsoft.AspNetconstruct.Http;
	using Microsoft.Extensions.Configuration;
	using Microsoft.Extensions.DependencyInjection;

	namespace WebAPILambda
	{
		public class Startup
		{							
			public void ConfigureServices(IServiceCollection services)
			{
				services.AddControllers();
			}

			public void Configure(IApplicationBuilder app, IWebHostEnvironment env)
			{
				/**
				 * @klotho::expose {
				 *  id = "my-gateway"
				 *  target = "public"
				 * }
				 */
				app.UseEndpoints(endpoints => endpoints.MapControllers());
			}
		}
	}`

	type gateway struct {
		Name   string
		Routes []routeMethodPath
	}
	type srcFile struct {
		Path    string
		Content string
	}

	parseDep := func(dep string) graph.Edge[construct.Construct] {
		parts := strings.Split(dep, ":")
		return graph.Edge[construct.Construct]{
			Source:      &types.Gateway{Name: parts[0]},
			Destination: &types.ExecutionUnit{Name: parts[1]},
		}
	}

	tests := []struct {
		name             string
		units            map[string][]srcFile
		expectedGateways []gateway
		expectedDeps     []string
	}{
		{
			name: "Routes added using Map<VERB>() are detected",
			units: map[string][]srcFile{
				"main": {
					{
						Path: "Startup.cs",
						Content: `
						using Microsoft.AspNetconstruct.Builder;
						using Microsoft.AspNetconstruct.Hosting;
						using Microsoft.AspNetconstruct.Http;
						using Microsoft.AspNetconstruct.Routing;

						namespace WebAPILambda
						{
							public class Startup
							{
								public void Configure(IApplicationBuilder app, IWebHostEnvironment env)
								{
									/**
									 * @klotho::expose {
									 *  id = "my-gateway"
									 *  target = "public"
									 * }
									 */
									app.UseEndpoints(endpoints =>
									{
										endpoints.Map("/any-path", () => "ok");
										endpoints.MapGet("/path", () => "ok");
										endpoints.MapPut("/path", () => "ok");
										endpoints.MapPost("/path", () =>  "ok");
										endpoints.MapDelete("/other-path", () => "ok");
									});
								}
							}
						}
						`,
					},
				},
			},
			expectedGateways: []gateway{
				{
					Name: "my-gateway",
					Routes: []routeMethodPath{
						{Verb: types.VerbAny, Path: "/any-path"},
						{Verb: types.VerbGet, Path: "/path"},
						{Verb: types.VerbPost, Path: "/path"},
						{Verb: types.VerbPut, Path: "/path"},
						{Verb: types.VerbDelete, Path: "/other-path"},
					},
				},
			},
			expectedDeps: []string{
				"my-gateway:main",
			},
		},
		{
			name: "Controller routes are added if AddControllers() and MapControllers() are invoked on valid startup class",
			units: map[string][]srcFile{
				"unit1-MapControllers": {
					{
						Path: "Startup.cs",
						Content: `
						using Microsoft.AspNetconstruct.Builder;
						using Microsoft.AspNetconstruct.Hosting;
						using Microsoft.AspNetconstruct.Http;
						using Microsoft.Extensions.Configuration;
						using Microsoft.Extensions.DependencyInjection;
						using Microsoft.Extensions.Hosting;

						namespace WebAPILambda
						{
							public class Startup
							{							
								public void ConfigureServices(IServiceCollection services)
								{
									services.AddControllers();
								}
						
								public void Configure(IApplicationBuilder app, IWebHostEnvironment env)
								{
									/**
									 * @klotho::expose {
									 *  id = "gateway1"
									 *  target = "public"
									 * }
									 */
									app.UseEndpoints(endpoints =>
									{
										endpoints.MapGet("/local-route", () => "ok");
										endpoints.MapControllers();
									});
								}
							}
						}
						`,
					},
					{
						Path: "controller1.cs",
						Content: `
						using System;
						using Microsoft.AspNetconstruct.Mvc;
						
						namespace WebAPILambda.Controllers
						{
													
							[Route("api/[controller]")]
							public class Controller1Controller
							{
								[HttpGet]
								public string Get()
								{
									return "ok";
								}
							}
						}
						`,
					},
				},
				"unit2-no-MapControllers": {
					{
						Path: "Startup.cs",
						Content: `
						using Microsoft.AspNetconstruct.Builder;
						using Microsoft.AspNetconstruct.Hosting;
						using Microsoft.AspNetconstruct.Http;
						using Microsoft.Extensions.Configuration;
						using Microsoft.Extensions.DependencyInjection;
						using Microsoft.Extensions.Hosting;

						namespace WebAPILambda
						{
							public class Startup
							{							
								public void ConfigureServices(IServiceCollection services)
								{
									services.AddControllers();
								}
						
								public void Configure(IApplicationBuilder app, IWebHostEnvironment env)
								{
									/**
									 * @klotho::expose {
									 *  id = "gateway2"
									 *  target = "public"
									 * }
									 */
									app.UseEndpoints(endpoints =>
									{
										endpoints.MapGet("/local-route", () => "ok");
									});
								}
							}
						}
						`,
					},
					{
						Path: "controller1.cs",
						Content: `
						using System;
						using Microsoft.AspNetconstruct.Mvc;
						
						namespace WebAPILambda.Controllers
						{
													
							[Route("api/[controller]")]
							public class Controller1Controller
							{
								[HttpGet]
								public string Get()
								{
									return "ok";
								}
							}
						}
						`,
					},
				},
				"unit3-no-AddControllers": {
					{
						Path: "Startup.cs",
						Content: `
						using Microsoft.AspNetconstruct.Builder;
						using Microsoft.AspNetconstruct.Hosting;
						using Microsoft.AspNetconstruct.Http;
						using Microsoft.Extensions.Configuration;
						using Microsoft.Extensions.DependencyInjection;
						using Microsoft.Extensions.Hosting;

						namespace WebAPILambda
						{
							public class Startup
							{
								public void Configure(IApplicationBuilder app, IWebHostEnvironment env)
								{
									/**
									 * @klotho::expose {
									 *  id = "gateway3"
									 *  target = "public"
									 * }
									 */
									app.UseEndpoints(endpoints =>
									{
										endpoints.MapControllers();
									});
								}
							}
						}
						`,
					},
					{
						Path: "controller1.cs",
						Content: `
						using System;
						using Microsoft.AspNetconstruct.Mvc;
						
						namespace WebAPILambda.Controllers
						{
													
							[Route("api/[controller]")]
							public class Controller1Controller
							{
								[HttpGet]
								public string Get()
								{
									return "ok";
								}
							}
						}
						`,
					},
				},

				"unit4-private-startup-class": {
					{
						Path: "Startup.cs",
						Content: `
						using Microsoft.AspNetconstruct.Builder;
						using Microsoft.AspNetconstruct.Hosting;
						using Microsoft.AspNetconstruct.Http;
						using Microsoft.Extensions.Configuration;
						using Microsoft.Extensions.DependencyInjection;
						using Microsoft.Extensions.Hosting;

						namespace WebAPILambda
						{
							private class Startup
							{							
								public void ConfigureServices(IServiceCollection services)
								{
									services.AddControllers();
								}
						
								public void Configure(IApplicationBuilder app, IWebHostEnvironment env)
								{
									/**
									 * @klotho::expose {
									 *  id = "gateway1"
									 *  target = "public"
									 * }
									 */
									app.UseEndpoints(endpoints =>
									{
										endpoints.MapGet("/local-route", () => "ok");
										endpoints.MapControllers();
									});
								}
							}
						}
						`,
					},
					{
						Path: "controller1.cs",
						Content: `
						using System;
						using Microsoft.AspNetconstruct.Mvc;
						
						namespace WebAPILambda.Controllers
						{
													
							[Route("api/[controller]")]
							public class Controller1Controller
							{
								[HttpGet]
								public string Get()
								{
									return "ok";
								}
							}
						}
						`,
					},
				},
			},
			expectedGateways: []gateway{
				{
					Name: "gateway1",
					Routes: []routeMethodPath{
						{Verb: types.VerbGet, Path: "/local-route"},
						{Verb: types.VerbGet, Path: "/api/controller1"},
					},
				},
				{
					Name: "gateway2",
					Routes: []routeMethodPath{
						{Verb: types.VerbGet, Path: "/local-route"},
					},
				},
				{
					Name: "gateway3",
					Routes: []routeMethodPath{
						{Verb: types.VerbAny, Path: "/"},
						{Verb: types.VerbAny, Path: "/:proxy*"},
					},
				},
			},
			expectedDeps: []string{
				"gateway1:unit1-MapControllers",
				"gateway2:unit2-no-MapControllers",
				"gateway3:unit3-no-AddControllers",
			},
		},
		{
			name: "All routing attributes applied to an action are handled",
			units: map[string][]srcFile{
				"main": {
					{
						Path:    "Startup.cs",
						Content: controllerMappingStartupClass,
					},
					{
						Path: "MyController.cs",
						Content: `
						using Microsoft.AspNetconstruct.Mvc;
						[Route("/api/[controller]")]
						public class MyController {
							[Route("child")]
							[Route("/root/child")]
							[AcceptVerbs("GET")]
							[AcceptVerbs("DELETE", Route="/del")]
							[HttpGet]
							[HttpDelete,HttpPut]
							public void action() {}
						}
						`,
					},
				},
			},
			expectedGateways: []gateway{
				{
					Name: "my-gateway",
					Routes: []routeMethodPath{
						{Verb: types.VerbGet, Path: "/api/my"},
						{Verb: types.VerbDelete, Path: "/api/my"},
						{Verb: types.VerbPut, Path: "/api/my"},
						{Verb: types.VerbGet, Path: "/api/my/child"},
						{Verb: types.VerbGet, Path: "/root/child"},
						{Verb: types.VerbDelete, Path: "/del"},
					},
				},
			},
			expectedDeps: []string{
				"my-gateway:main",
			},
		},
		{
			name: "Optional or default last path params result in required and optional routes",
			units: map[string][]srcFile{
				"main": {
					{
						Path:    "Startup.cs",
						Content: controllerMappingStartupClass,
					},
					{
						Path: "MyController.cs",
						Content: `
						using Microsoft.AspNetconstruct.Mvc;
						public class MyController {
							[Route("/required/{optional?}")]
							[Route("/api/required/{default=value}")]
							[Route("/api/{x=default}/{y=default}/{default=value}")]
							[AcceptVerbs("GET")]
							public void action() {}
						}
						`,
					},
				},
			},
			expectedGateways: []gateway{
				{
					Name: "my-gateway",
					Routes: []routeMethodPath{
						{Verb: types.VerbGet, Path: "/required"},
						{Verb: types.VerbGet, Path: "/required/:optional"},
						{Verb: types.VerbGet, Path: "/api/required"},
						{Verb: types.VerbGet, Path: "/api/required/:default"},
						{Verb: types.VerbGet, Path: "/api/:rest*"},
					},
				},
			},
			expectedDeps: []string{
				"my-gateway:main",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)
			result := construct.NewConstructGraph()
			for uName, files := range tt.units {
				unit := &types.ExecutionUnit{
					Executable: types.NewExecutable(),
					Name:       uName,
				}
				for _, f := range files {
					sf, err := types.NewSourceFile(f.Path, strings.NewReader(f.Content), Language)
					if !assert.NoError(err) {
						return
					}
					unit.AddSourceFile(sf)
				}
				result.AddConstruct(unit)
			}
			expose := Expose{}
			err := expose.Transform(&types.InputFiles{}, &types.FileDependencies{}, result)
			if !assert.NoError(err) {
				return
			}

			gateways := construct.GetConstructsOfType[*types.Gateway](result)
			assert.Equal(len(tt.expectedGateways), len(gateways))

			sort.Slice(gateways, func(i, j int) bool {
				return gateways[i].Name < gateways[j].Name
			})
			sort.Slice(tt.expectedGateways, func(i, j int) bool {
				return tt.expectedGateways[i].Name < tt.expectedGateways[j].Name
			})

			for _, gw := range gateways {
				sort.Slice(gw.Routes, func(i, j int) bool {
					if gw.Routes[i].Path == gw.Routes[j].Path {
						return gw.Routes[i].Verb < gw.Routes[j].Verb
					} else {
						return gw.Routes[i].Path < gw.Routes[j].Path
					}
				})
			}

			for _, gw := range tt.expectedGateways {
				sort.Slice(gw.Routes, func(i, j int) bool {
					if gw.Routes[i].Path == gw.Routes[j].Path {
						return gw.Routes[i].Verb < gw.Routes[j].Verb
					} else {
						return gw.Routes[i].Path < gw.Routes[j].Path
					}
				})
			}

			for i, expectedGw := range tt.expectedGateways {
				if i >= len(gateways) {
					break
				}

				var aRoutes []routeMethodPath
				for _, r := range gateways[i].Routes {
					aRoutes = append(aRoutes, routeMethodPath{
						Verb: r.Verb,
						Path: r.Path,
					})
				}
				assert.Equal(len(expectedGw.Routes), len(aRoutes))
				assert.ElementsMatch(expectedGw.Routes, aRoutes)
			}
			depsArr := result.ListDependencies()

			assert.Equal(len(tt.expectedDeps), len(depsArr))
			var eDeps []graph.Edge[construct.Construct]
			for _, dep := range tt.expectedDeps {
				eDeps = append(eDeps, parseDep(dep))
			}
			sort.Slice(eDeps, func(i, j int) bool {
				if eDeps[i].Source.Id() == eDeps[j].Source.Id() {
					return eDeps[i].Destination.Id().String() < eDeps[j].Destination.Id().String()
				} else {
					return eDeps[i].Source.Id().String() < eDeps[j].Source.Id().String()
				}
			})
			sort.Slice(depsArr, func(i, j int) bool {
				if depsArr[i].Source.Id() == depsArr[j].Source.Id() {
					return depsArr[i].Destination.Id().String() < depsArr[j].Destination.Id().String()
				} else {
					return depsArr[i].Source.Id().String() < depsArr[j].Source.Id().String()
				}
			})

			for i, eDep := range eDeps {
				if i >= len(depsArr) {
					break
				}
				aDep := depsArr[i]
				assert.Equal(eDep.Source.Id(), aDep.Source.Id())
				assert.Equal(eDep.Destination.Id(), aDep.Destination.Id())
			}
		})
	}
}
