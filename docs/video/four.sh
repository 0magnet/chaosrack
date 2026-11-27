#!/bin/sh
# show.mp4: a showcase on the second drop of "Four", one look a bar (144 BPM),
# drawn by the rack on the page's black, modulated as the page modulates.
# Usage: four.sh "NumberNin6 - Four.mp3", run in an empty directory; it writes
# each shot there and keeps any already made, so a rerun redoes only what is missing.
set -eu
CR=${CR:-chaosrack}
F=${1:?usage: four.sh path/to/Four.mp3}
FPS=60
B=1.6666667
Z=0,0,0,0,0,0,0,0
BASS=1,1,0,0,0,0,0,0
at() { awk -v b=$B -v k="$1" 'BEGIN{printf "%.4f", 216+k*b}'; }
deg() { awk -v d="$1" 'BEGIN{printf "%.6f", d*3.14159265358979/180}'; }
SCOPEPOSE="$(deg 178.299),$(deg 76.1078),$(deg 221.034)"
SCOPE="--set gradient-source=4 --set gradient-colors=5 --set stereo-win=250 --set stereo-width=0 --set stereo-span=2.75"
AIZ="--set aizawa-dt=0.006 --set aizawa-a=0.85 --set aizawa-b=0.654 --set aizawa-c=0.623 --set aizawa-d=3.07 --set aizawa-e=0.236 --set aizawa-f=0.02 --set gradient-source=3 --set gradient-colors=4 --set rainbow-freq=14.0022 --set trail-slider=15277"
SPECT="--set spect-min=-30 --set spect-max=25 --set spect-ovl=75 --set camera-zoom=40"

# shot NAME K0 K1 W H PRE MODEL ARGS...: bars K0..K1 of the drop, rendered from
# PRE seconds earlier so trails and spectrograms are full, then trimmed.
shot() {
	n=$1 k0=$2 k1=$3 w=$4 h=$5 pre=$6 m=$7
	if [ -f "$n.mp4" ]; then echo "shot $n (kept)"; return 0; fi
	shift 7
	s=$(at "$k0") e=$(at "$k1")
	from=$(awk -v s="$s" -v p="$pre" 'BEGIN{print s-p}')
	len=$(awk -v s="$s" -v e="$e" -v p="$pre" 'BEGIN{print e-s+p}')
	if ! "$CR" render --engine rack --model "$m" --audio-file "$F" --audio-start "$from" --audio "$len" \
		--width "$w" --height "$h" --fps $FPS --crf 14 "$@" -o raw-$n.mp4 >log-$n.txt 2>&1; then
		echo "shot $n FAILED"; tail -3 log-$n.txt; return 1
	fi
	ffmpeg -v error -y -ss "$pre" -i raw-$n.mp4 -t "$(awk -v s="$s" -v e="$e" 'BEGIN{print e-s}')" -an \
		-c:v libx264 -crf 14 -preset medium -pix_fmt yuv420p $n.mp4
	echo "shot $n"
}

W=1920 H=1080
shot 01 -10 -9 $W $H 4 aizawa $AIZ --angle 0,0,0 --mod aizawa-d=st~$Z~0.15~swing &
shot 02 -9 -8 $W $H 4 halvorsen --set auto-rotate=on --set gradient-source=4 --set gradient-colors=8 --mod halvorsen-a=st~$Z~0.2~swing &
shot 03 -8 -7 $W $H 24 thomas --set auto-rotate=on --set gradient-source=4 --set gradient-colors=10 --set bg-visual=spectrogram &
shot 04 -7 -6 $W $H 4 clifford --set gradient-source=1 --set gradient-colors=10 --mod clifford-a=st~$Z~0.15~swing &
shot 05 -6 -5 $W $H 4 dejong --set gradient-source=0 --set gradient-colors=9 &
shot 06 -5 -4 $W $H 4 lorenz --set auto-rotate=on --set gradient-source=4 --set gradient-colors=8 --mod lorenz-r=st~$Z~0.05~swing &
wait
shot 07 -4 -3 $W $H 24 torus --set auto-rotate=on --set gradient-source=4 --set gradient-colors=4 &
shot 08 -3 -2 $W $H 4 rossler --set auto-rotate=on --set gradient-source=4 --set gradient-colors=9 &
shot 09 -2 -1 $W $H 2 polar --set gradient-source=4 --set gradient-colors=10 --set camera-zoom=70 &
shot 10 -1 1 $W $H 1 stereo $SCOPE --set stereo-axes=1 --angle "$SCOPEPOSE" &
shot 11 1 2 $W $H 24 spectrogram $SPECT --set gradient-colors=5 &
shot 12 2 3 $W $H 2 takens --set auto-rotate=on --set gradient-source=4 --set gradient-colors=8 --mod view-zoom=st~$BASS~0.5~push &
wait
shot 13 3 4 $W $H 24 lorenz --set auto-rotate=on --set gradient-source=4 --set gradient-colors=10 --set bg-visual=spectrogram --mod view-zoom=st~$BASS~0.4~push &
shot 14 4 5 $W $H 4 aizawa $AIZ --set auto-rotate=on --mod aizawa-d=st~$Z~0.2~swing &
MSRATE=$(awk -v b=$B 'BEGIN{printf "%.6f", (3.14159265358979/4)/(3*b)}')
shot 15 5 6 $W $H 1 stereo $SCOPE --set stereo-axes=3 --angle "$(deg 178.299),$(deg 76.1078),$(awk -v r="$MSRATE" 'BEGIN{printf "%.6f", 221.034*3.14159265358979/180 - r*3}')" --set rotation-controls-z=$MSRATE &
shot 16 6 7 $W $H 4 halvorsen --set auto-rotate=on --set gradient-source=4 --set gradient-colors=4 --mod halvorsen-a=st~$Z~0.3~flip &
shot 17 7 8 $W $H 24 thomas --set gradient-source=4 --set gradient-colors=10 --set bg-visual=spectrogram --mod view-spiny=st~$Z~0.5~push &
wait

# The finale: nine models at once, a third of the frame each.
w=640 h=360
shot g1 8 12 $w $h 4 aizawa $AIZ --set auto-rotate=on --mod aizawa-d=st~$Z~0.2~swing &
shot g2 8 12 $w $h 1 stereo $SCOPE --set stereo-axes=1 --angle "$SCOPEPOSE" &
shot g3 8 12 $w $h 24 spectrogram $SPECT --set gradient-colors=5 &
shot g4 8 12 $w $h 2 takens --set auto-rotate=on --set gradient-source=4 --set gradient-colors=8 &
shot g5 8 12 $w $h 4 lorenz --set auto-rotate=on --set gradient-source=4 --set gradient-colors=8 --mod lorenz-r=st~$Z~0.05~swing &
wait
shot g6 8 12 $w $h 4 clifford --set gradient-source=1 --set gradient-colors=10 --mod clifford-a=st~$Z~0.15~swing &
shot g7 8 12 $w $h 24 torus --set auto-rotate=on --set gradient-source=4 --set gradient-colors=4 &
shot g8 8 12 $w $h 4 halvorsen --set auto-rotate=on --set gradient-source=4 --set gradient-colors=4 --mod halvorsen-a=st~$Z~0.3~flip &
shot g9 8 12 $w $h 24 spectrogram $SPECT --set gradient-colors=10 &
wait
ffmpeg -v error -y $(for i in 1 2 3 4 5 6 7 8 9; do printf -- '-i g%s.mp4 ' $i; done) \
	-filter_complex "[0][1][2][3][4][5][6][7][8]xstack=inputs=9:layout=0_0|w0_0|w0+w1_0|0_h0|w0_h0|w0+w1_h0|0_h0+h3|w0_h0+h3|w0+w1_h0+h3" \
	-c:v libx264 -crf 14 -preset medium -pix_fmt yuv420p 18.mp4
echo "shot 18 (grid)"

for i in 01 02 03 04 05 06 07 08 09 10 11 12 13 14 15 16 17 18; do [ -f $i.mp4 ] || { echo "missing shot $i"; exit 1; }; done
S0=$(at -10) S1=$(at 12)
DUR=$(awk -v a="$S0" -v b="$S1" 'BEGIN{print b-a}')
FO=$(awk -v d="$DUR" -v b=$B 'BEGIN{print d-b}')
list=""
for i in 01 02 03 04 05 06 07 08 09 10 11 12 13 14 15 16 17 18; do list="$list -i $i.mp4"; done
ffmpeg -v error -y $list -ss "$S0" -t "$DUR" -i "$F" \
	-filter_complex "$(for i in $(seq 0 17); do printf '[%s]' $i; done)concat=n=18:v=1:a=0,fade=t=in:d=0.4,fade=t=out:st=$FO:d=$B[v];[18]afade=t=in:d=0.3,afade=t=out:st=$FO:d=$B[a]" \
	-map "[v]" -map "[a]" -c:v libx264 -crf 16 -preset slow -pix_fmt yuv420p -c:a aac -b:a 256k -movflags +faststart show.mp4
echo "show.mp4: $DUR s"
