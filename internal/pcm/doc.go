// Package pcm decodes audio files to PCM samples, for measuring them. It reads
// FLAC, MP3, and the PCM and floating-point audio of WAV and AIFF; other
// formats return [ErrUnsupported]. It is pure Go, and only ever reads.
//
// A [Stream] gives the audio in chunks, one plane of samples per channel,
// scaled so that full scale is 1.
package pcm
