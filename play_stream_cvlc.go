/*
TRemote plugin play_stream_cvlc implements an internet radio player.
This is useful sample code, demonstrating how things can be implemented in the
context of a TRemote plugin. This is also a very useful standalone internet
radio player, that is reliable and fun to use.
play_stream_cvlc is bound to a single button. A short press starts the first
station from a given list or internet radio stations. From this moment forward
audio will be streamed until it is stopped externally.
When the same button is pressed again, playback will skip to the next station.
If the same button is long-pressed (at least 500ms), audio playback will skip
back to the previous station. play_stream_cvlc executes cvlc to play back the
audio stream. The names and the URL's of the radio stations are listed in the
mapping.txt file and are handed over by TRemote service via rcs.StrArray.
*/
package main

import (
	"bufio"
	"html"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mehrvarz/log"
	"github.com/mehrvarz/tremote_plugin"
)

var (
	logm           log.Logger
	instanceNumber int
	argIndex       = -1
	lock_Mutex     sync.Mutex

	pluginname      = "play_stream_cvlc"
	AudioControl    = "amixer set Master -q"
	AudioPlayer     = "cvlc --play-and-exit"
	AudioPlayerKill = "pkill -TERM vlc"
)

func init() {
	instanceNumber = 0
}

/*
Action() is the main entry point for any TRemote plugin. We need to make
sure Action() will always return super quickly no matter what. This is why
we start new goroutines for opertations that take more time. The first thing
we must do is to figure out if we are coping with a short or a long press
event. Once this is determined, we call actioncall() with true (for
longpress) or false (for shortpress) to have it play the next station, or
the previous one. We use a Mutex to prevent interruption during the short
period of time Action() is active.
*/
func Action(log log.Logger, pid int, longpress bool, pressedDuration int64, rcs *tremote_plugin.RemoteControlSpec, ph tremote_plugin.PluginHelper) error {
	var lock_Mutex sync.Mutex
	lock_Mutex.Lock()
	logm = log

	strArray := rcs.StrArray
	if longpress {
		strArray = rcs.StrArraylong
	}

	//logm.Debugf("%s pid=%d PIdLastPressed=%d",pluginname,pid,*ph.PIdLastPressed)
	if pid != *ph.PIdLastPressed {
		// this button is different than the previous button: start with the first strArray element
		argIndex = len(strArray) - 1
	}
	*ph.PIdLastPressed = pid

	//logm.Debugf("%s instanceNumber=%d",pluginname,instanceNumber)
	if instanceNumber == 0 {
		// may run something here only on very 1st run
		// read config.txt for AudioControl, AudioPlayer, AudioPlayerKill
		readConfig("")
	}
	instanceNumber++

	if pressedDuration == 0 {
		// button just pressed, is not yet released
		//logm.Debugf("%s pressedDuration==0 pid=%d %d",pluginname,pid,(*ph.PLastPressActionDone)[pid])
		go func() {
			// let's see if button is still pressed after LongPressDelay MS
			time.Sleep(tremote_plugin.LongPressDelay * time.Millisecond)
			if (*ph.PLastPressedMS)[pid] > 0 {
				// button is still pressed; this is a longpress; let's take care of it
				(*ph.PLastPressActionDone)[pid] = true
				//logm.Debugf("%s pressedDuration==0 pid=%d %d",pluginname,pid,(*ph.PLastPressActionDone)[pid])
				actioncall(true, strArray, pid, ph)
			}
		}()

	} else {
		// button has been released -> short press
		if !(*ph.PLastPressActionDone)[pid] {
			(*ph.PLastPressActionDone)[pid] = true
			//logm.Debugf("%s short press pid=%d %d",pluginname,pid,(*ph.PLastPressActionDone)[pid])
			go func() {
				actioncall(false, strArray, pid, ph)
			}()
		}
	}

	lock_Mutex.Unlock()
	return nil
}

func actioncall(longpress bool, strArray []string, pid int, ph tremote_plugin.PluginHelper) error {
	var reterr error
	var audioStreamName, audioStreamSource string
	var lock_Mutex sync.Mutex
	lock_Mutex.Lock()

	instance := instanceNumber

	if longpress {
		//logm.Debugf("%s (%d) start long-press",pluginname,instance)
		argIndex--
		if argIndex < 0 {
			argIndex = len(strArray) - 1
		}
		audioStreamName, audioStreamSource = getStreamNameAndSource(strArray[argIndex])
		logm.Infof("%s long-press audioStreamName=%s", pluginname, audioStreamName)
	} else {
		// short press
		//logm.Debugf("%s (%d) start short-press",pluginname,instance)
		argIndex++
		if argIndex >= len(strArray) {
			argIndex = 0
		}
		audioStreamName, audioStreamSource = getStreamNameAndSource(strArray[argIndex])
		logm.Infof("%s short-press audioStreamName=%s", pluginname, audioStreamName)
	}

	if *ph.PluginIsActive {
		// player is already running
		logm.Debugf("%s (%d) on start another instance already running", pluginname, instance)
		// stop older instance
		if *ph.StopAudioPlayerChan != nil {
			logm.Debugf("%s (%d) kill other instance...", pluginname, instance)
			*ph.StopAudioPlayerChan <- true
			// wait for other instance to exec AudioPlayerKill (so it won't kill our new vlc instance) and terminate
			time.Sleep(200 * time.Millisecond)
		} else {
			logm.Warningf("%s (%d) no StopAudioPlayerChan exist to kill other instance", pluginname, instance)
		}
	} else {
		// no instance of our player is currently running so we don't know if any audio playback is ongoing
		// stop whatever audio may currently be playing
		logm.Debugf("%s (%d) on start no PluginIsActive -> StopCurrentAudioPlayback()", pluginname, instance)
		ph.StopCurrentAudioPlayback()
		time.Sleep(200 * time.Millisecond)
	}

	// we are running now
	logm.Debugf("%s (%d) set PluginIsActive", pluginname, instance)
	var ourStopAudioPlayerChan chan bool
	if *ph.StopAudioPlayerChan == nil {
		// this allows parent to stop playback
		ourStopAudioPlayerChan = make(chan bool)
		*ph.StopAudioPlayerChan = ourStopAudioPlayerChan
	}
	*ph.PluginIsActive = true

	ph.PrintInfo(html.EscapeString(audioStreamName))
	startTime := time.Now()

	cmd := AudioPlayer + " \"" + audioStreamSource + "\""
	logm.Infof("%s exec cmd [%s]", pluginname, cmd)
	cmd_audio := exec.Command("sh", "-c", cmd)
	if cmd_audio == nil {
		logm.Warningf("%s cmd_audio==nil after exec.Command()", pluginname)
		*ph.PluginIsActive = false
		if *ph.StopAudioPlayerChan != nil {
			*ph.StopAudioPlayerChan = nil
		}

	} else {
		stdout, err := cmd_audio.StdoutPipe()
		if err != nil {
			logm.Warningf("%s StdoutPipe err %s", pluginname, err.Error())
		}

		stderr, err := cmd_audio.StderrPipe()
		if err != nil {
			logm.Warningf("%s StderrPipe err %s", pluginname, err.Error())
		}

		if stdout != nil {
			go func() {
				outputReader := bufio.NewReader(stdout)
				for {
					if cmd_audio == nil {
						logm.Debugf("%s cmd_audio ended", pluginname)
						break
					}
					if stdout == nil || outputReader == nil {
						logm.Debugf("%s cmd_audio stdout closed", pluginname)
						break
					}
					outputStr, err := outputReader.ReadString('\n')
					if err != nil {
						if err.Error() != "EOF" && strings.Index(err.Error(), "file already closed") < 0 {
							logm.Warningf("%s stdout err: %s", pluginname, err.Error())
						}
						break
					}
					strlen := len(outputStr)
					if strlen > 0 {
						if !strings.HasPrefix(outputStr, "Command Line Interface initialized") &&
							!strings.HasPrefix(outputStr, "> Shutting down") &&
							!strings.HasPrefix(outputStr, "VLC media player") {
							// stripping trailing '\n'
							logm.Infof("%s stdout:%s", pluginname, outputStr[:strlen-1])
						}
					}
				}
			}()
		}

		if stderr != nil {
			go func() {
				errReader := bufio.NewReader(stderr)
				for {
					if cmd_audio == nil {
						break
					}
					stderrStr, err := errReader.ReadString('\n')
					if err != nil {
						if err.Error() != "EOF" && strings.Index(err.Error(), "file already closed") < 0 {
							logm.Warningf("%s stderr err: %s", pluginname, err.Error())
						}
						break
					}
					strlen := len(stderrStr)
					if strlen > 0 {
						// don't display these msgs from cvlc
						if !strings.Contains(stderrStr, "no suitable services discovery module") &&
							!strings.Contains(stderrStr, "using the dummy interface") &&
							!strings.Contains(stderrStr, "core interface") &&
							!strings.Contains(stderrStr, "core libvlc") &&
							!strings.Contains(stderrStr, "core playlist") &&
							!strings.Contains(stderrStr, "dbus interface") &&
							!strings.Contains(stderrStr, "lua interface") {
							// stripping trailing '\n'
							logm.Debugf("%s stderr:%s", pluginname, stderrStr[:strlen-1])
						}
					}
				}
			}()
		}

		reterr = cmd_audio.Start() // will return immediately
		if reterr != nil {
			// process didn't start
			logm.Warningf("%s cmd_audio.Start() didn't start", pluginname)
			cmd_audio = nil // must stop stdout/stderr threads
			*ph.PluginIsActive = false
			if *ph.StopAudioPlayerChan != nil {
				*ph.StopAudioPlayerChan = nil
			}
			errString := reterr.Error()
			logm.Warningf("%s process.Start err=[%s]", pluginname, errString)
			ph.PrintInfo("")
		} else {
			logm.Debugf("%s (%d) cmd_audio.Start() OK; waiting...", pluginname, instance)

			// attach this process to a WaitGroup
			var wg sync.WaitGroup
			wg.Add(1)

			// we now start two threads to cope with abort-requests and cvlc eventually ending
			// any of the two can trigger first
			go func() {
				// our 1st thread is waiting for a stop event

				time.Sleep(500 * time.Millisecond)
				audioVolumeUnmute(instance)

				<-*ph.StopAudioPlayerChan
				// received stop event either from: new instance, cvlc has just ended, or event was sent from outside

				if *ph.StopAudioPlayerChan != nil && *ph.StopAudioPlayerChan == ourStopAudioPlayerChan {
					*ph.StopAudioPlayerChan = nil
					*ph.PluginIsActive = false
				}
				if cmd_audio == nil {
					logm.Debugf("%s (%d) playback has finish", pluginname, instance)
				} else {
					logm.Debugf("%s (%d) playback being killed", pluginname, instance)
					exe_cmd(AudioPlayerKill, false, false, instance)
				}
				wg.Done() // signaling to waitgroup that this process is done
			}()
			go func() {
				// our 2nd thread is waiting for cvlc to end
				// cvlc running...
				reterr = cmd_audio.Wait()
				// cvlc has ended

				errString := "-"
				if reterr != nil {
					errString = reterr.Error()
				}
				durationMS := time.Now().Sub(startTime) / time.Millisecond
				// if durationMS <200:  possibly cvlc not installed?
				logm.Debugf("%s (%d) cmd_audio.Wait() ended after %d ms %s", pluginname, instance, durationMS, errString)
				cmd_audio = nil
				stdout = nil
				stderr = nil
				if *ph.StopAudioPlayerChan != nil && *ph.StopAudioPlayerChan == ourStopAudioPlayerChan {
					*ph.StopAudioPlayerChan <- true
				}
			}()
		}
	}

	lock_Mutex.Unlock()
	return reterr
}

func getStreamNameAndSource(entry string) (string, string) {
	audioStreamSource := entry
	audioStreamName := ""
	idxEqual := strings.Index(entry, "=")
	idxSlash := strings.Index(entry, "/")
	if idxEqual >= 0 && (idxSlash < 0 || idxSlash > idxEqual) {
		audioStreamName = entry[:idxEqual]
		audioStreamSource = entry[idxEqual+1:]
	}
	return audioStreamName, audioStreamSource
}

func audioVolumeUnmute(instance int) error {
	logm.Debugf("%s (%d) audioVolumeUnmute()", pluginname, instance)
	return exe_cmd(AudioControl+" on", true, false, instance)
}

func exe_cmd(cmd string, logErr bool, logStdout bool, instance int) error {
	logm.Debugf("%s (%d) exe_cmd: sh [%s]", pluginname, instance, cmd)
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil && logErr {
		// not fatal
		logm.Warningf("%s (%d) exe_cmd [%s] err=%s", pluginname, instance, cmd, err.Error())
	}

	if out != nil && logStdout {
		if len(out) > 0 {
			logm.Infof("%s (%d) exe_cmd out=[%s]", pluginname, instance, out)
		}
	}
	return err
}

func readConfig(path string) int {
	pathfile := "config.txt"
	if len(path) > 0 {
		pathfile = path + "/config.txt"
	}

	file, err := os.Open(pathfile)
	if err != nil {
		logm.Infof("readConfig from "+pathfile+" failed: %s", err.Error())
		return 0 // not fatal, we can do without config.txt
	}
	defer file.Close()

	logm.Infof("readConfig from %s", pathfile)
	linecount := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		pound := strings.Index(line, "#")
		if pound >= 0 {
			//logm.Infof("readConfig found # at pos %d",pound)
			line = line[:pound]
		}
		if line != "" {
			line = strings.TrimSpace(line)
		}
		if line != "" {
			//logm.Infof("readConfig line: ["+line+"]")
			linetokens := strings.Split(line, "=")
			//logm.Infof("readConfig tokens: [%v]",linetokens)
			if len(linetokens) >= 2 {
				key := strings.TrimSpace(linetokens[0])
				value := strings.TrimSpace(linetokens[1])
				logm.Infof("readConfig key=%s val=%s", key, value)
				linecount++

				switch key {
				case "audiocontrol":
					AudioControl = value
				case "audioplayer":
					AudioPlayer = value
				case "audioplayerkill":
					AudioPlayerKill = value
				}
			}
		}
	}
	return linecount
}
