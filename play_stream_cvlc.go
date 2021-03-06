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
	"fmt"
	"bufio"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"runtime"

	"github.com/mehrvarz/log"
	"github.com/mehrvarz/tremote_plugin"
)

var (
	logm               log.Logger
	instanceNumber     int
	lock_Mutex         sync.Mutex
	waitingForOlderInstanceToStop = false
	argIndex           = -1
	audioStreamName    string
	audioStreamSource  string

	pluginname         = "play_stream_cvlc"
	AudioPlayer        = "cvlc --play-and-exit"
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
func Action(log log.Logger, pid int, longpress bool, pressedDuration int64, homedir string, rcs *tremote_plugin.RemoteControlSpec, ph tremote_plugin.PluginHelper, wg *sync.WaitGroup) error {
	var lock_Mutex sync.Mutex
	lock_Mutex.Lock()
	logm = log

	if instanceNumber == 0 {
		// may do things here only on 1st run
		// read config.txt for AudioPlayer
		readConfig(homedir)
	}
	instanceNumber++

	ph.HostCmd("ScreenPower","on")

	strArray := rcs.StrArray
	if longpress {
		strArray = rcs.StrArraylong
	}

	logm.Debugf("%s pid=%d PIdLastPressed=%d",pluginname,pid,*ph.PIdLastPressed)
	if pid != *ph.PIdLastPressed {
		// this button is different than the previous button: start with the first strArray element
		argIndex = len(strArray) - 1
	}
	*ph.PIdLastPressed = pid

	if pressedDuration==0 && !longpress {
		// button just pressed, is not yet released
		//logm.Debugf("%s pressedDuration==0 pid=%d %d",pluginname,pid,(*ph.PLastPressActionDone)[pid])
		go func() {
			// let's see if this becomes a longpress if pressed for tremote_plugin.LongPressDelay ms
			var msWaited time.Duration = 0
			for msWaited < tremote_plugin.LongPressDelay {
				time.Sleep(50 * time.Millisecond)
				msWaited += 50
				if (*ph.PLastPressActionDone)[pid] {
					// button has been taken care of
					break
				}
				if (*ph.PLastPressedMS)[pid] == 0 {
					// button is no longer pressed
					break
				}
				// button still pressed
			}
			// time is up or button released or button taken care of

			if (*ph.PLastPressedMS)[pid] > 0 && !(*ph.PLastPressActionDone)[pid] {
				// button is still pressed; os this is a longpress; let's take care of it
				(*ph.PLastPressActionDone)[pid] = true
				//logm.Debugf("%s pressedDuration==0 pid=%d %d",pluginname,pid,(*ph.PLastPressActionDone)[pid])
				actioncall(true, strArray, pid, ph, wg)
			}
		}()

	} else {
		// button has been released -> short press
		if (*ph.PLastPressActionDone)[pid] {
			// this button event has already been taken care of
		} else {
			(*ph.PLastPressActionDone)[pid] = true
			//logm.Debugf("%s short press pid=%d %d",pluginname,pid,(*ph.PLastPressActionDone)[pid])
			go func() {
				actioncall(false, strArray, pid, ph, wg)
			}()
		}
	}

	lock_Mutex.Unlock()
	return nil
}

func actioncall(longpress bool, strArray []string, pid int, ph tremote_plugin.PluginHelper, wg *sync.WaitGroup) {
	var lock_Mutex	sync.Mutex
	lock_Mutex.Lock()

	wg.Add(1)
	defer func() {
		if err := recover(); err != nil {
			wg.Done()
 			logm.Errorf("%s panic=%s", pluginname, err)
			buf := make([]byte, 1<<16)
			runtime.Stack(buf, true)
 			logm.Errorf("%s stack=\n%s", pluginname, buf)
	   }
	}()

	instance := instanceNumber

	if longpress {
		//logm.Debugf("%s (%d) start long-press",pluginname,instance)
		argIndex--
		if argIndex < 0 {
			argIndex = len(strArray) - 1
		}
		audioStreamName, audioStreamSource = getStreamNameAndSource(strArray[argIndex])
		logm.Infof("%s (%d) long-press audioStreamName=%s argIndex=%d", pluginname, instance, audioStreamName, argIndex)
	} else {
		// short press
		//logm.Debugf("%s (%d) start short-press",pluginname,instance)
		argIndex++
		if argIndex >= len(strArray) {
			argIndex = 0
		}
		audioStreamName, audioStreamSource = getStreamNameAndSource(strArray[argIndex])
		logm.Infof("%s (%d) short-press audioStreamName=%s argIndex=%d", pluginname, instance, audioStreamName,argIndex)
	}

	if waitingForOlderInstanceToStop {
		// an older instance of this plugin is already waiting for an even older instance to stop
		// we likely have too many overlapping actioncall() instances: giving up on this new instance
		logm.Debugf("%s (%d) exit on waitingForOlderInstanceToStop",pluginname,instance)

		// this won't update quickly enough
		//infostring := fmt.Sprintf("%s %d/%d",audioStreamName,argIndex+1,len(strArray))
		//ph.PrintInfo(infostring)

		// we updated audioStreamName + audioStreamSource and this is all we need to do - exit
		lock_Mutex.Unlock()
		return
	}




	if *ph.StopAudioPlayerChan!=nil {
		// an instance of our player is currently active
		waitingForOlderInstanceToStop = true
		lock_Mutex.Unlock()
		logm.Debugf("%s (%d) stopping other instance...",pluginname,instance)
		*ph.StopAudioPlayerChan <- true
		time.Sleep(200 * time.Millisecond)
	} else {
		// No instance of our player is currently active. There may be some other audio playing instance.
		// Stop whatever audio player may currently be active.
		waitingForOlderInstanceToStop = true
		lock_Mutex.Unlock()
		logm.Debugf("%s (%d) on start no audio Plugin active -> StopCurrentAudioPlayback()",pluginname,instance)
		ph.StopCurrentAudioPlayback()		// calling tools.Stop_current_stream()
		time.Sleep(200 * time.Millisecond)	// not sure this is needed
	}

	var ourStopAudioPlayerChan chan bool
	if *ph.StopAudioPlayerChan == nil {
		// this allows parent to stop playback
		ourStopAudioPlayerChan = make(chan bool)
		*ph.StopAudioPlayerChan = ourStopAudioPlayerChan
	}
	waitingForOlderInstanceToStop = false

	infostring := fmt.Sprintf("%s %d/%d",audioStreamName,argIndex+1,len(strArray))
	logm.Debugf("%s (%d) info: %s",pluginname,instance,infostring)
	ph.PrintInfo(infostring)
	startTime := time.Now()
	logm.Infof("%s play stream [%s]", pluginname, audioStreamSource)
	cmd := AudioPlayer + " \"" + audioStreamSource + "\""
	logm.Debugf("%s exec cmd [%s]", pluginname, cmd)
	cmd_audio := exec.Command("sh", "-c", cmd)
	if cmd_audio == nil {
		logm.Warningf("%s cmd_audio==nil after exec.Command()", pluginname)
		if *ph.StopAudioPlayerChan != nil {
			*ph.StopAudioPlayerChan = nil
		}
		wg.Done()

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
						logm.Debugf("%s outputReader cmd_audio ended", pluginname)
						break
					}
					if stdout == nil || outputReader == nil {
						logm.Debugf("%s outputReader cmd_audio stdout closed", pluginname)
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

		err = cmd_audio.Start() // will return immediately
		if err != nil {
			// process didn't start
			logm.Warningf("%s cmd_audio.Start() didn't start", pluginname)
			cmd_audio = nil // must stop stdout/stderr threads
			if *ph.StopAudioPlayerChan != nil {
				*ph.StopAudioPlayerChan = nil
			}
			logm.Warningf("%s process.Start err=[%s]", pluginname, err.Error())
			ph.PrintInfo("")
			wg.Done()
		} else {
			logm.Debugf("%s (%d) cmd_audio.Start() OK; waiting...", pluginname, instance)

			// we now start two goroutine to cope with stop-requests and cvlc eventually ending
			// any of the two can trigger first
			go func() {
				// our 1st goroutine is waiting for a stop event

				// mute may be on; turn in off to be sure;  
				time.Sleep(500 * time.Millisecond)
				ph.HostCmd("AudioMute","off")

				// wait for a stop-request
				<-*ph.StopAudioPlayerChan
				// received stop-request either from: new instance, cvlc has just ended, or event was sent from outside

				if *ph.StopAudioPlayerChan != nil && *ph.StopAudioPlayerChan == ourStopAudioPlayerChan {
					*ph.StopAudioPlayerChan = nil
					ourStopAudioPlayerChan = nil
				}
				if cmd_audio == nil {
					logm.Debugf("%s (%d) playback has finish", pluginname, instance)
					//ph.PrintInfo("")
				} else {
					logm.Debugf("%s (%d) playback being killed", pluginname, instance)
					// this will wake our 2nd goroutine by killing the cvlc process
					ph.StopCurrentAudioPlayback()
				}
				if wg!=nil {
					logm.Debugf("%s (%d) wg.Done()", pluginname, instance)
					wg.Done() // this process is done
					wg=nil
				} else {
					logm.Debugf("%s (%d) wg==nil", pluginname, instance)
				}
			}()
			go func() {
				// our 2nd goroutine is waiting for cvlc to end
				// cvlc running...
				err := cmd_audio.Wait()
				// cvlc has ended
				if ourStopAudioPlayerChan!=nil {
					// TODO: avoid; this may be an old instance, removing info of new instance
					// we have not been killed by a newer instance; the audio stream just finished by itself
					ph.PrintInfo("")
				}

				errString := "-"
				if err != nil {
					errString = err.Error()
				}
				durationMS := time.Now().Sub(startTime) / time.Millisecond
				// if durationMS <200:  possibly cvlc not installed?
				logm.Debugf("%s (%d) cmd_audio.Wait() ended after %d ms %s", pluginname, instance, durationMS, errString)

				cmd_audio = nil
				stdout = nil
				stderr = nil
				if *ph.StopAudioPlayerChan != nil && *ph.StopAudioPlayerChan == ourStopAudioPlayerChan {
					// stop our other goroutine (in case we still own the channel)
					*ph.StopAudioPlayerChan <- true
					// the other goroutine will decr waitgroup
				} else {
					if wg!=nil {
						logm.Debugf("%s (%d) goroutine2 wg.Done()", pluginname, instance)
						wg.Done() // this process is done
						wg=nil
					} else {
						logm.Debugf("%s (%d) goroutine2 wg==nil", pluginname, instance)
					}
				}
			}()
		}
	}
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

func readConfig(path string) int {
	pathfile := "config.txt"
	if len(path) > 0 {
		pathfile = path + "/config.txt"
	}

	file, err := os.Open(pathfile)
	if err != nil {
		logm.Infof("%s readConfig from %s failed: %s", pluginname,pathfile,err.Error())
		return 0 // not fatal, we can do without config.txt
	}
	defer file.Close()

	logm.Debugf("%s readConfig from %s", pluginname, pathfile)
	linecount := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		pound := strings.Index(line, "#")
		if pound >= 0 {
			//logm.Debugf("%s readConfig found # at pos %d",pluginname,pound)
			line = line[:pound]
		}
		if line != "" {
			line = strings.TrimSpace(line)
		}
		if line != "" {
			//logm.Debugf("%s readConfig line: [%s]",pluginname,line)
			linetokens := strings.SplitN(line, "=", 2)
			//logm.Debugf("%s readConfig tokens: [%v]",pluginname,linetokens)
			if len(linetokens) >= 2 {
				key := strings.TrimSpace(linetokens[0])
				value := strings.TrimSpace(linetokens[1])
				linecount++

				switch key {
				case "audioplayer":
					logm.Debugf("%s readConfig key=[%s] val=[%s]", pluginname, key, value)
					AudioPlayer = value
				}
			}
		}
	}
	return linecount
}
