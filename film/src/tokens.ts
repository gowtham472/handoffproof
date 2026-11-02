import {loadFont} from '@remotion/google-fonts/PlusJakartaSans';

// Plus Jakarta Sans (the SAAP film's font), loaded locally by Remotion so
// every rendered frame matches. Fall back to system sans anywhere a glyph
// is missing.
const loaded = loadFont('normal', {
	weights: ['400', '500', '600', '700', '800'],
	subsets: ['latin'],
});

export const FONT = loaded.fontFamily;
export const BPM = 117.5; // measured from the track (autocorrelation)
export const BEAT = 60 / BPM; // seconds per beat
export const BAR = BEAT * 4; // seconds per bar

// b(bar, beat) -> seconds on the music grid.
export const b = (bar: number, beat = 0, fraction = 0) =>
	(bar * 4 + beat + fraction) * BEAT;
