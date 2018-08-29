# TRemote plugin play_stream_cvlc - Internet radio streamer

TRemote is a service for ARM based Linux computers. It lets you remote control *things* on these kind of machines, specifically over Bluetooth. There is no limit to what you can remote control. You can access a list of predefined actions, you can execute executables and shell scripts, you can issue http request, and you can invoke your own or 3rd party native code plugins.

This repository contains the complete Go source code of a remote control plugin application. You can use this plugin as-is. You can also use it as a template to implement similar or extended functionality.

TRemote plugin **play_stream_cvlc** implements a streaming radio application.
This is useful sample code, demonstrating how things can be implemented in the 
context of a TRemote plugin. This is also a very useful application 
that works reliably and is fun to use.

This plugin makes use of cvlc. You may need to install cvlc via "apt install vlc-nox".


# Building the plugin

TRemote plugins are based on Go Modules. You need to use [Go v1.11](https://dl.google.com/go/go1.11.linux-armv6l.tar.gz) (direct dl link for linux-armv6l) to build this plugin. Before you start make sure your "go version" command returns "go version go1.11 linux/arm".

After cloning this repository enter the following command to build the plugin:

```
CGO_ENABLED=1 go build -buildmode=plugin play_stream_cvlc.go
```
This will create the "play_stream_cvlc.so" binary. Copy the binary over to your TRemote folder, add a mapping entry like the one shown below to your mapping.txt file and restart the TRemote service. You can now invoke your plugin functionality via a Bluetooh remote control.

If you intend to modify this plugin or create a similar one, you should also take a look at the [TRemote plugin base package](https://github.com/mehrvarz/tremote_plugin).

You can use the runtrp tool to run TRemote plugins from the command line. Download runtrp for [linux-armv6l](https://github.com/mehrvarz/tremote_plugin/tree/master/bin.linux.ARM6) and for [linux-amd64](https://github.com/mehrvarz/tremote_plugin/tree/master/bin.linux.AMD64).


# Button mapping

The following entry in "mapping.txt" will bind the radio streaming plugin to a specific button (P1) and hand over the station name and URL (TheJazzGroove.org=http://199.180.75.26:80/stream):


```
P1, JazzGroove, play_stream|TheJazzGroove.org=http://199.180.75.26:80/stream
```

You can also setup a (long) list of radio stations:

```
P1, JazzRadio, play_stream|TheJazzGroove.org=http://199.180.75.26:80/stream|UK1940s=http://1940sradio1.co.uk:8100/1|Secklow105.5=http://31.25.191.64:8000/;?t=1528915624|BBC=http://bbcmedia.ic.llnwd.net/stream/bbcmedia_lrberk_mf_p|Radio Swiss Jazz=http://www.radioswissjazz.ch/live/aacp.m3u|Smooth
```

When you press the configured button again, the next radio stations will get played. A longpress will skip one station back.
You can step through the list of stations round-robin in both directions.

You can also confgure multple buttons for different sets of radio stations:

```
P1, JazzRadio, play_stream|TheJazzGroove.org=http://199.180.75.26:80/stream|UK1940s=http://1940sradio1.co.uk:8100/1|Secklow105.5=http://31.25.191.64:8000/;?t=1528915624|BBC=http://bbcmedia.ic.llnwd.net/stream/bbcmedia_lrberk_mf_p|Radio Swiss Jazz=http://www.radioswissjazz.ch/live/aacp.m3u|Smooth
P2, TalkRadio, play_stream|DLF=http://st01.dlf.de/dlf/01/104/ogg/stream.ogg|DLK=http://st02.dlf.de/dlf/02/104/ogg/stream.ogg|RadioBERLIN 88,8=http://www.radioberlin.de/live.pls|SRF 4 News Swiss=http://stream.srg-ssr.ch/drs4news/mp3_128.m3u
```

Note that a plugin does not know anything about remote controls, about Bluetooth or how a button event is delivered to it. It only cares about the implementation of the response action. The mapping file bindes the two sides together.


