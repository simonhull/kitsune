import { describe, expect, it } from 'vitest';
import { springParams } from '../choreography';
import { scaleShadow, ShadowMix } from './shadow';

const params = springParams({ duration: 420, bounce: 0 });
const hero = 'rgba(0, 0, 0, 0.5) 0px 40px 90px 0px';
const poster = 'rgba(0, 0, 0, 0.55) 0px 60px 120px 0px';
const card = 'none';

describe('scaleShadow', () => {
	it('scales lengths and leaves colors alone', () => {
		expect(scaleShadow(hero, 2)).toBe('rgba(0, 0, 0, 0.5) 0.00px 80.00px 180.00px 0.00px');
	});

	it('leaves no shadow as none', () => {
		expect(scaleShadow(card, 2)).toBe('none');
	});
});

describe('ShadowMix', () => {
	it('starts as exactly the source and ends as exactly the target', () => {
		const mix = new ShadowMix(hero, poster);
		expect(mix.opacities).toEqual([1, 0]);
		for (let frame = 0; frame < 120; frame += 1) {
			mix.step(params, 1 / 60);
		}
		expect(mix.opacities.at(1)).toBeCloseTo(1, 3);
	});

	it('never leaves a gap: the two opacities always add up to one', () => {
		const mix = new ShadowMix(hero, poster);
		for (let frame = 0; frame < 20; frame += 1) {
			mix.step(params, 1 / 60);
			const [from, to] = mix.opacities;
			expect(from + to).toBeCloseTo(1, 6);
		}
	});

	it('turns around from where it is, without a jump', () => {
		const mix = new ShadowMix(hero, poster);
		mix.step(params, 0.12);
		const before = mix.weight.value;
		mix.aim(hero);
		expect(mix.weight.value).toBe(before);
		expect(mix.weight.target).toBe(0);
	});

	it('swaps a third shadow in on the side that shows least', () => {
		const mix = new ShadowMix(hero, poster);
		mix.step(params, 0.5);
		mix.aim(card);
		expect(mix.shadows).toEqual([card, poster]);
		expect(mix.weight.target).toBe(0);
	});
});
