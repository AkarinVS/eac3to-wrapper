// eac3to wrapper program.
//
// This program wraps eac3to and fixes long-standing bugs:
// (1) unable to decompress zlib deflated PGS subtitles in mkv.
//     It will transparently uses mkvmerge to extract *.sup.
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AkarinVS/eac3to-wrapper/mkv"
)

const prefix = "eac3to-wrapper"

var (
	// path to essential executables
	mkvExtractPath string
	mkvMergePath   string
	eac3toPath     string
)

// findExe locates the path for executable of given name.
// It will try the executable's directory and then relative paths in altdir.
func findExe(name string, altdir ...string) (path string) {
	self, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	selfi, err := os.Stat(self)
	if err != nil {
		log.Fatal(err)
	}
	dir := filepath.Dir(self)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, path := range append([]string{"."}, altdir...) {
		fn := filepath.Join(dir, path, name)
		// XXX: we only determines if the file exists, and not bother to check its FileMode.
		fi, err := os.Stat(fn)
		if err == nil && !fi.IsDir() {
			// also skip ourselves to avoid endless loops.
			if os.SameFile(selfi, fi) {
				log.Printf("skipped %s while looking for %s", fn, name)
				continue
			}
			return fn
		}
	}
	log.Printf("unable to locate %s from %s", name, dir)
	return ""
}

// checkEnv checks if the execution environment is sane.
func checkEnv() {
	if os.Getenv("EAC3TO_WRAPPER_DEV") == "" {
		logf := fmt.Sprintf("%s-%s.log", prefix, time.Now().Format("20060102"))
		for _, dir := range []string{"./log", "../../log"} {
			fn := filepath.Join(dir, logf)
			fd, err := os.OpenFile(fn, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				log.SetOutput(fd)
				fmt.Fprintf(os.Stderr, "log file at %s\n", fn)
				break
			}
		}
	}
	// There are three different places eac3to-wrapper could be placed:
	// (1) during development, at the same directory with mkv{extract,merge} and eac3to.
	// (2) under tools/eac3to.
	// (3) at the same directory as OKEGui.exe.
	mkvExtractPath = findExe("mkvextract", "../mkvtoolnix", "tools/mkvtoolnix")
	mkvMergePath = findExe("mkvmerge", "../mkvtoolnix", "tools/mkvtoolnix")
	eac3toPath = findExe("eac3to", "../eac3to", "tools/eac3to", "../eac3to.real")
	if mkvExtractPath == "" || mkvMergePath == "" || eac3toPath == "" {
		log.Fatal("unable to locate essential programs, abort")
	}
	log.Printf("located %s %s %s", mkvExtractPath, mkvMergePath, eac3toPath)
}

// getMkvTracks uses `mkvmerge -J` on the given mkvf file and parses the result
// into mkv.Info.
func getMkvTracks(mkvf string) (*mkv.Info, error) {
	cmd := exec.Command(mkvMergePath, "-J", mkvf)
	log.Printf("running %v", cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	info, err := mkv.ParseInfo(out)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// getTrackMapping processes mkv.Info and generate a eac3to track to mkvtoolnix
// track mapping table.
//
// mkvtoolnix always uses the 0-based physical track ID, but eac3to's 1-based
// track ID is more complicated:
// 1. video first, then audio, and finally subtitle.
// 2. within each group, sort by the track number property of the track.
func getTrackMapping(info *mkv.Info) []int {
	tracks := append([]*mkv.Track(nil), info.Tracks...)
	sort.Slice(tracks, func(i, j int) bool {
		ti, tj := tracks[i].Type(), tracks[j].Type()
		if ti < tj {
			return true
		} else if ti == tj {
			return tracks[i].Number < tracks[j].Number
		}
		return false
	})
	mapping := make([]int, len(tracks)+1)
	mapping[0] = -1 // poison the invalid eac3to track 0
	for i, trk := range tracks {
		mapping[i+1] = trk.Id
	}
	return mapping
}

func run(prog string, args []string) error {
	cmd := exec.Command(prog, args...)
	log.Printf("running %v", cmd)
	// XXX: workaround golang/go#45914.
	outpipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	defer outpipe.Close()
	errpipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	defer errpipe.Close()
	go io.Copy(os.Stdout, outpipe)
	go io.Copy(os.Stderr, errpipe)
	return cmd.Run()
}

// fileHasSuffix determines if a filename has a given suffix, taking into
// consideration of platform case (in)sensitivity.
func fileHasSuffix(filename string, suffix string) bool {
	if runtime.GOOS == "windows" {
		filename = strings.ToLower(filename)
		suffix = strings.ToLower(suffix)
	}
	return filepath.Ext(filename) == suffix
}

// Type Track represents a track extraction with eac3to.
type Track struct {
	Id       int
	Filename string
}

var extractTrackRe = regexp.MustCompile(`^([1-9][0-9]*):(.*)$`)

// parseEac3toArgs approximately parses eac3to args and see if workaround is
// necessary.
// Specially, if it's extracting from mkv, then mkvFile will be the input mkv
// file, and tracks will be all the *.sup subtitle tracks.
// Finally, filtered arguments will be returned as newArgs.
func parseEac3toArgs(args []string) (newArgs []string, mkvFile string, tracks []Track) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		newArgsPrev := newArgs
		newArgs = append(newArgs, arg)
		if arg[0] != '-' && arg[0] != '+' {
			// check for mkv filename
			if fileHasSuffix(arg, ".mkv") && mkvFile == "" {
				mkvFile = arg
			}
			// check for track extraction
			if m := extractTrackRe.FindStringSubmatch(arg); len(m) == 3 {
				var trk Track
				trk.Id, _ = strconv.Atoi(m[1])
				trk.Filename = m[2]
				if trk.Filename == "" && len(args) > i+1 {
					trk.Filename = args[i+1]
					newArgs = append(newArgs, trk.Filename)
					i++
				}
				// only intercept *.sup extractions from *.mkv
				if mkvFile != "" && fileHasSuffix(trk.Filename, ".sup") {
					tracks = append(tracks, trk)
					newArgs = newArgsPrev // remove this extraction
					continue
				}
			}
		}
	}
	log.Printf("translated %q, mkv %q, tracks %v", newArgs, mkvFile, tracks)
	return
}

func main() {
	log.SetPrefix(prefix + ": ")
	log.SetFlags(0)

	checkEnv()
	log.Printf("command line %q", os.Args)

	nargs, mkvf, tracks := parseEac3toArgs(os.Args[1:])
	if mkvf != "" && len(tracks) > 0 {
		info, err := getMkvTracks(mkvf)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("%#v", *info)
		mapping := getTrackMapping(info)
		log.Printf("track mapping from eac3to to mkvtoolnix: %v", mapping)

		// build mkvextract args
		extArgs := []string{mkvf, "tracks"}
		for _, trk := range tracks {
			extArgs = append(extArgs, fmt.Sprintf("%d:%s", mapping[trk.Id], trk.Filename))
		}
		err = run(mkvExtractPath, extArgs)
		if err != nil {
			log.Fatal(err)
		}
	}

	err := run(eac3toPath, nargs)
	if err != nil {
		log.Fatal(err)
	}
}
