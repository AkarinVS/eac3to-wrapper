eac3to-wrapper
==============

eac3to-wrapper aims to fix eac3to's long standing [bug 288](http://bugs.madshi.net/view.php?id=288).

To build, first install Go from [golang.org/dl](https://golang.org/dl/), then:
```bat
go build
```
and copy the executable to the directory where OKEGui.exe resides.
(Note: this requires modified OKEGui, or you can follow the [Testing procedure](#Testing) to install
without a modified OKEGui.).


Testing
-------

To test in your vanilla OKEGui installation, do this, assuming eac3to-wrapper.exe is at D:/.
```bat
cd path\to\OKEGui
cd tools
ren eac3to eac3to.real
mkdir eac3to
copy d:\eac3to-wrapper.exe eac3to\eac3to.exe
```
To restore your environment, just delete the newly created tools\eac3to directory and then
restore tools\eac3to.real to tools\eac3to.

When invoked with OKEGui, the program will append to log files under OKEGui/log/eac3to-wrapper-YYMMDD.log.

Limitations
-----------

It only recognizes the following forms:

1. `eac3to input.mkv TID1:out1.flac TID2:out2.sup ...`
2. `eac3to input.mkv TID1: out1.flac TID2: out2.sup ...`

All unrecognized forms will be passed through to original eac3to in verbatim.
