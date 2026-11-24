package desktop

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// testCopy is an Integration whose entries live in a throwaway registry key
// and folder, so tests never touch the real startup list or Start menu.
func testCopy(t *testing.T, exe string, args ...string) *Integration {
	t.Helper()
	root := fmt.Sprintf(`Software\SortwiseTest%d`, time.Now().UnixNano())
	t.Cleanup(func() {
		for _, key := range []string{root + `\Run`, root + `\Approved`, root + `\Link\shell\open\command`, root + `\Link\shell\open`, root + `\Link\shell`, root + `\Link\DefaultIcon`, root + `\Link`, root} {
			_ = registry.DeleteKey(registry.CURRENT_USER, key)
		}
	})
	return &Integration{
		exe:         exe,
		args:        args,
		runKey:      root + `\Run`,
		approvedKey: root + `\Approved`,
		valueName:   "Sortwise",
		shortcut:    filepath.Join(t.TempDir(), "Sortwise.lnk"),
		linkKey:     root + `\Link`,
	}
}

func startupValue(t *testing.T, i *Integration) string {
	t.Helper()
	key, err := registry.OpenKey(registry.CURRENT_USER, i.runKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, _ := key.GetStringValue(i.valueName)
	return value
}

func TestFirstRunAddsBothAndLaterRunsFollowTheExe(t *testing.T) {
	i := testCopy(t, `C:\Apps\Sortwise\sortwise.exe`, "--port", "8790", "--data-dir", `D:\My Library`)
	if state := i.State(); !state.Available || state.StartMenu || state.Startup {
		t.Fatalf("before the first run: %+v", state)
	}
	if err := i.Refresh(true); err != nil {
		t.Fatal(err)
	}
	if state := i.State(); !state.StartMenu || !state.Startup {
		t.Fatalf("after the first run: %+v", state)
	}
	want := `"C:\Apps\Sortwise\sortwise.exe" --no-open --port 8790 --data-dir "D:\My Library"`
	if got := startupValue(t, i); got != want {
		t.Fatalf("starts with %s, want %s", got, want)
	}
	target, args, err := readShortcut(i.shortcut)
	if err != nil || !strings.EqualFold(target, i.exe) || args != `--port 8790 --data-dir "D:\My Library"` {
		t.Fatalf("shortcut points at %q %q (%v)", target, args, err)
	}

	// Moved: the next run points both at the new place.
	i.exe = `E:\Sortwise\sortwise.exe`
	if err := i.Refresh(false); err != nil {
		t.Fatal(err)
	}
	if got := startupValue(t, i); !strings.HasPrefix(got, `"E:\Sortwise\sortwise.exe"`) {
		t.Fatalf("startup entry did not follow the exe: %s", got)
	}
	if target, _, _ := readShortcut(i.shortcut); !strings.EqualFold(target, i.exe) {
		t.Fatalf("shortcut did not follow the exe: %s", target)
	}
}

func TestWhatYouTurnOffStaysOff(t *testing.T) {
	i := testCopy(t, `C:\Apps\sortwise.exe`)
	if err := i.Refresh(true); err != nil {
		t.Fatal(err)
	}
	if err := i.SetStartMenu(false); err != nil {
		t.Fatal(err)
	}
	if err := i.SetStartup(false); err != nil {
		t.Fatal(err)
	}
	if err := i.Refresh(false); err != nil {
		t.Fatal(err)
	}
	if state := i.State(); state.StartMenu || state.Startup {
		t.Fatalf("a later run brought them back: %+v", state)
	}
	// Turning off what is already off is fine.
	if err := i.SetStartMenu(false); err != nil {
		t.Fatal(err)
	}
	if err := i.SetStartup(false); err != nil {
		t.Fatal(err)
	}
	if err := i.SetStartMenu(true); err != nil || !i.State().StartMenu {
		t.Fatalf("could not turn the shortcut back on: %v", err)
	}
}

func TestStartupTurnedOffInTaskManagerCountsAsOff(t *testing.T) {
	i := testCopy(t, `C:\Apps\sortwise.exe`)
	if err := i.SetStartup(true); err != nil {
		t.Fatal(err)
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, i.approvedKey, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	// Task Manager's flag for a startup app turned off.
	if err := key.SetBinaryValue(i.valueName, []byte{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	key.Close()
	if i.State().Startup {
		t.Fatal("an app turned off in Task Manager showed as starting")
	}
	// A later run keeps it off; turning it on here turns it on there too.
	if err := i.Refresh(false); err != nil || i.State().Startup {
		t.Fatalf("a later run changed Task Manager's choice: %v", err)
	}
	if err := i.SetStartup(true); err != nil || !i.State().Startup {
		t.Fatalf("turning it on did not clear Task Manager's flag: %v", err)
	}
}

func TestTheLinkStartsThisCopyQuietly(t *testing.T) {
	i := testCopy(t, `C:\Apps\sortwise.exe`, "--port", "8790")
	if err := i.RegisterLink(); err != nil {
		t.Fatal(err)
	}
	read := func(path, name string) string {
		key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		defer key.Close()
		value, _, err := key.GetStringValue(name)
		if err != nil {
			t.Fatalf("%s %q: %v", path, name, err)
		}
		return value
	}
	read(i.linkKey, "URL Protocol") // marks it as a link
	if got := read(i.linkKey+`\shell\open\command`, ""); got != `"C:\Apps\sortwise.exe" --no-open --port 8790` {
		t.Fatalf("the link runs %s", got)
	}
	// The link follows the exe, whatever the shortcut and startup entry do.
	i.exe = `D:\sortwise.exe`
	if err := i.RegisterLink(); err != nil {
		t.Fatal(err)
	}
	if got := read(i.linkKey+`\shell\open\command`, ""); got != `"D:\sortwise.exe" --no-open --port 8790` {
		t.Fatalf("the link did not follow the exe: %s", got)
	}
}

func TestTemporaryBuildsAreNotSetUp(t *testing.T) {
	// Tests run from a temporary build, like `go run`.
	if ForThisCopy(nil) != nil {
		t.Fatal("a temporary build would add a shortcut that soon points nowhere")
	}
	if !inside(`C:\Temp\go-build1\exe\sortwise.exe`, `C:\Temp`) || inside(`C:\Temperature\sortwise.exe`, `C:\Temp`) {
		t.Fatal("inside misread a folder")
	}
}

// readShortcut returns where a shortcut points and the arguments it passes.
func readShortcut(path string) (target, args string, err error) {
	err = withShellLink(func(link, file comObject) error {
		location, _ := windows.UTF16PtrFromString(path)
		const readOnly = 0
		if result := file.call(persistLoad, uintptr(unsafe.Pointer(location)), readOnly); failed(result) {
			return fmt.Errorf("could not open the shortcut (0x%08x)", uint32(result))
		}
		runtime.KeepAlive(location)
		pathBuffer := make([]uint16, windows.MAX_PATH)
		link.call(linkGetPath, uintptr(unsafe.Pointer(&pathBuffer[0])), uintptr(len(pathBuffer)), 0, 0)
		argsBuffer := make([]uint16, 1024)
		link.call(linkGetArguments, uintptr(unsafe.Pointer(&argsBuffer[0])), uintptr(len(argsBuffer)))
		target, args = windows.UTF16ToString(pathBuffer), windows.UTF16ToString(argsBuffer)
		return nil
	})
	return target, args, err
}
