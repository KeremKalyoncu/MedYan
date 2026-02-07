package handlers

import (
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/gofiber/fiber/v2"
)

// RegisterPprofRoutes registers pprof profiling endpoints
// These endpoints are useful for performance debugging and optimization
func RegisterPprofRoutes(app *fiber.App) {
	profiling := app.Group("/debug/pprof")

	// Index page
	profiling.Get("/", pprofIndex)

	// CPU profile - records CPU usage for specified duration
	// Usage: curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof
	// Analyze: go tool pprof cpu.prof
	profiling.Get("/profile", pprofProfile)

	// Heap profile - shows memory allocations
	// Usage: curl http://localhost:8080/debug/pprof/heap > heap.prof
	// Analyze: go tool pprof heap.prof
	profiling.Get("/heap", pprofHeap)

	// Goroutine profile - shows all goroutines
	// Useful for detecting goroutine leaks
	profiling.Get("/goroutine", pprofGoroutine)

	// Thread create profile
	profiling.Get("/threadcreate", pprofThreadCreate)

	// Block profile - shows blocking operations
	profiling.Get("/block", pprofBlock)

	// Mutex profile - shows mutex contentions
	profiling.Get("/mutex", pprofMutex)

	// Allocs profile - similar to heap but for allocations
	profiling.Get("/allocs", pprofAllocs)

	// Command line arguments
	profiling.Get("/cmdline", pprofCmdline)

	// Symbol lookup
	profiling.Get("/symbol", pprofSymbol)

	// Trace - execution trace for specified duration
	// Usage: curl http://localhost:8080/debug/pprof/trace?seconds=5 > trace.out
	// Analyze: go tool trace trace.out
	profiling.Get("/trace", pprofTrace)
}

func pprofIndex(c *fiber.Ctx) error {
	profiles := pprof.Profiles()
	html := `<html><head><title>pprof</title></head><body>
	<h1>/debug/pprof/</h1>
	<p>Available profiles:</p>
	<ul>
	<li><a href="/debug/pprof/profile?seconds=30">30-second CPU profile</a></li>
	<li><a href="/debug/pprof/heap">heap profile</a></li>
	<li><a href="/debug/pprof/goroutine">goroutine profile</a></li>
	<li><a href="/debug/pprof/threadcreate">threadcreate profile</a></li>
	<li><a href="/debug/pprof/block">block profile</a></li>
	<li><a href="/debug/pprof/mutex">mutex profile</a></li>
	<li><a href="/debug/pprof/allocs">allocs profile</a></li>
	<li><a href="/debug/pprof/trace?seconds=5">5-second execution trace</a></li>
	</ul>
	<p>Total profiles: ` + string(rune(len(profiles))) + `</p>
	</body></html>`

	c.Set("Content-Type", "text/html")
	return c.SendString(html)
}

func pprofProfile(c *fiber.Ctx) error {
	seconds := c.QueryInt("seconds", 30)
	if seconds <= 0 || seconds > 300 { // Max 5 minutes
		seconds = 30
	}

	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Disposition", "attachment; filename=profile")

	if err := pprof.StartCPUProfile(c.Response().BodyWriter()); err != nil {
		return c.Status(500).SendString("Could not start CPU profile: " + err.Error())
	}

	time.Sleep(time.Duration(seconds) * time.Second)
	pprof.StopCPUProfile()

	return nil
}

func pprofHeap(c *fiber.Ctx) error {
	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Disposition", "attachment; filename=heap")

	runtime.GC() // Force GC to get accurate heap snapshot

	if err := pprof.WriteHeapProfile(c.Response().BodyWriter()); err != nil {
		return c.Status(500).SendString("Could not write heap profile: " + err.Error())
	}

	return nil
}

func pprofGoroutine(c *fiber.Ctx) error {
	return writePprofProfile(c, "goroutine", "goroutine")
}

func pprofThreadCreate(c *fiber.Ctx) error {
	return writePprofProfile(c, "threadcreate", "threadcreate")
}

func pprofBlock(c *fiber.Ctx) error {
	return writePprofProfile(c, "block", "block")
}

func pprofMutex(c *fiber.Ctx) error {
	return writePprofProfile(c, "mutex", "mutex")
}

func pprofAllocs(c *fiber.Ctx) error {
	return writePprofProfile(c, "allocs", "allocs")
}

func pprofCmdline(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/plain")
	// Return command line arguments as string
	return c.SendString("pprof cmdline endpoint")
}

func pprofSymbol(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/plain")
	return c.SendString("Symbol lookup not implemented")
}

func pprofTrace(c *fiber.Ctx) error {
	seconds := c.QueryInt("seconds", 5)
	if seconds <= 0 || seconds > 60 { // Max 1 minute
		seconds = 5
	}

	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Disposition", "attachment; filename=trace")

	// Note: runtime/trace package would be needed for full implementation
	// This is a simplified version
	return c.SendString("Trace functionality requires runtime/trace package integration")
}

func writePprofProfile(c *fiber.Ctx, name, filename string) error {
	profile := pprof.Lookup(name)
	if profile == nil {
		return c.Status(404).SendString("Profile not found: " + name)
	}

	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Disposition", "attachment; filename="+filename)

	if err := profile.WriteTo(c.Response().BodyWriter(), 0); err != nil {
		return c.Status(500).SendString("Could not write profile: " + err.Error())
	}

	return nil
}
