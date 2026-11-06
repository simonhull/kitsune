import { describe, expect, it } from 'vitest';
import { Listen } from './listen.ts';

function listen(seconds: number, at = 0): Listen {
	return new Listen({ id: 't', durationMs: seconds * 1000 }, 0, at);
}

/** Plays from `from` to `to` in ticks of a second, and counts the ticks that made it count. */
function play(subject: Listen, from: number, to: number): number {
	let counts = 0;
	for (let seconds = from; seconds <= to; seconds += 1) {
		counts += subject.heard(seconds) ? 1 : 0;
	}
	return counts;
}

describe('Listen', () => {
	it('counts at half a track', () => {
		const subject = listen(100);
		expect(play(subject, 0, 49)).toBe(0);
		expect(subject.heard(50)).toBe(true);
	});

	it('counts at 4 minutes of a long one', () => {
		const subject = listen(3600);
		expect(play(subject, 0, 239)).toBe(0);
		expect(subject.heard(240)).toBe(true);
	});

	it('counts once', () => {
		expect(play(listen(100), 0, 100)).toBe(1);
	});

	it('never counts a track under 30 seconds, and does count one of exactly that', () => {
		expect(play(listen(29.9), 0, 29)).toBe(0);
		expect(play(listen(30), 0, 30)).toBe(1);
	});

	it('takes a throttled tick at its word', () => {
		expect(listen(100).heard(50)).toBe(true);
	});

	it('credits nothing for a jump forward', () => {
		const subject = listen(100);
		subject.jumped(100);
		expect(subject.heard(100)).toBe(false);
	});

	it('credits small seeks nothing either, however many', () => {
		const subject = listen(100);
		for (let seconds = 0; seconds <= 100; seconds += 2) {
			subject.jumped(seconds);
			expect(subject.heard(seconds)).toBe(false);
		}
	});

	it('adds up what was heard either side of a jump', () => {
		const subject = listen(100);
		expect(play(subject, 0, 30)).toBe(0);
		subject.jumped(60);
		expect(play(subject, 60, 79)).toBe(0);
		expect(subject.heard(80)).toBe(true);
	});

	it('counts going back over the same part: it was heard twice', () => {
		const subject = listen(100);
		expect(play(subject, 0, 30)).toBe(0);
		subject.jumped(0);
		expect(play(subject, 0, 20)).toBe(1);
	});

	it('hears nothing in an output coming back to where it was', () => {
		const subject = listen(100);
		expect(play(subject, 0, 40)).toBe(0);
		expect(subject.heard(0)).toBe(false);
		expect(subject.heard(20)).toBe(false);
		expect(subject.heard(40)).toBe(false);
		expect(play(subject, 41, 49)).toBe(0);
		expect(subject.heard(50)).toBe(true);
	});

	it('starts from where it is told', () => {
		const subject = listen(100, 40);
		expect(subject.heard(40)).toBe(false);
		expect(play(subject, 41, 89)).toBe(0);
		expect(subject.heard(90)).toBe(true);
	});
});
