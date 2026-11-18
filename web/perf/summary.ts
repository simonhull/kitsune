/**
 * Writes the whole table from samples gathered into `results`, as the suite's teardown does: CI
 * runs it once every shard is done, with each shard's samples downloaded there.
 */
import { writeReport } from './report.ts';

writeReport();
