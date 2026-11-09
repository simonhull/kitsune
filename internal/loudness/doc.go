// Package loudness measures audio the way ITU-R BS.1770-4 and EBU R 128 say,
// and turns the measurement into ReplayGain 2.0. It is pure: it takes PCM
// samples and returns numbers, and knows nothing of files, codecs or the store.
//
// # Measuring
//
// A [Meter] takes the audio of one track in chunks of any size ([Meter.Push])
// and gives its [Result] once ([Meter.Finish]). The result is the same however
// the audio was cut into chunks. Loudness is K-weighted and gated in 400 ms
// blocks with 75% overlap: blocks under -70 LUFS are dropped, then blocks more
// than 10 LU under the loudness of the rest. Loudness range follows EBU Tech
// 3342: 3 s windows, a -20 LU relative gate, and the 10th to 95th percentile.
// The true peak is read 4 times oversampled below 96 kHz and twice below
// 192 kHz, as BS.1770-4 Annex 2 asks.
//
// # Albums
//
// An album's loudness is that of its tracks' gated blocks pooled, as if they
// were one program, and never the average of their loudness. A [Histogram]
// holds a track's blocks compactly, to be stored and pooled later ([Pool]),
// without reading the audio again.
//
// # ReplayGain
//
// ReplayGain 2.0 plays everything at [Reference] LUFS: a gain is the reference
// less the loudness ([Gain]).
package loudness
