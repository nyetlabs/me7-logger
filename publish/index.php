<?php
$zips = glob(__DIR__.'/*-[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9].zip') ?: [];

// make stage passes corpus image names, and --out writes the joined table.
// A missing row or family fails the publish.
if (PHP_SAPI === 'cli') {
	$out = null;
	$names = [];
	foreach (array_slice($argv, 1) as $arg) {
		if (str_starts_with($arg, '--out=')) {
			$out = substr($arg, 6);
			continue;
		}
		$names[] = $arg;
	}
	$corpus = getenv('CORPUS');
	if ($corpus === false || $corpus === '') {
		fwrite(STDERR, "CORPUS is unset\n");
		exit(1);
	}
	$vehicles = loadVehicles(__DIR__.'/vehicles.yaml');
	$families = loadFamilies($corpus.'/corpus.tsv');
	$missing = 0;
	foreach ($names as $name) {
		if (!isset($vehicles[$name])) {
			fwrite(STDERR, "$name: no vehicle information\n");
			$missing = 1;
			continue;
		}
		if (familyOf($families, $name) === null) {
			fwrite(STDERR, "$name: no family in corpus.tsv\n");
			$missing = 1;
		}
	}
	if ($out !== null) {
		[$grades, $err] = loadGrades();
		if ($err !== '') {
			fwrite(STDERR, "$err\n");
			exit(1);
		}
		$joined = [];
		foreach ($vehicles as $name => $_) {
			$cells = joined($name, $vehicles, $families, $grades);
			if ($cells === null) {
				fwrite(STDERR, "$name: no family in corpus.tsv\n");
				$missing = 1;
				continue;
			}
			$joined[$name] = $cells;
		}
		if ($missing === 0 && file_put_contents($out, prettyJson($joined)) === false) {
			fwrite(STDERR, "$out: unwritable\n");
			exit(1);
		}
	}
	exit($missing);
}

$notes = [];
$notesFile = __DIR__.'/vehicles.json';
if (is_file($notesFile)) {
	$decoded = json_decode((string)file_get_contents($notesFile), true);
	if (is_array($decoded)) {
		$notes = $decoded;
	}
}

function loadVehicles(string $path): array {
	if (!function_exists('yaml_parse_file')) {
		fwrite(STDERR, "php yaml extension is required\n");
		exit(1);
	}
	$parsed = yaml_parse_file($path);
	if (!is_array($parsed)) {
		fwrite(STDERR, "$path: unreadable\n");
		exit(1);
	}
	$fields = ['engine', 'code', 'vehicle', 'year', 'gearbox', 'market'];
	$out = [];
	foreach ($parsed as $name => $row) {
		if (!is_array($row)) {
			fwrite(STDERR, "$path: $name: bad entry\n");
			exit(1);
		}
		$cells = array_fill_keys($fields, '');
		foreach ($row as $key => $value) {
			if (!is_string($key) || !array_key_exists($key, $cells)) {
				fwrite(STDERR, "$path: $name: unknown field $key\n");
				exit(1);
			}
			$cells[$key] = is_scalar($value) ? (string)$value : '';
		}
		$out[$name] = $cells;
	}
	return $out;
}

function loadFamilies(string $path): array {
	$lines = file($path, FILE_IGNORE_NEW_LINES);
	if ($lines === false) {
		fwrite(STDERR, "$path: unreadable\n");
		exit(1);
	}
	$header = explode("\t", (string)array_shift($lines));
	$nameCol = array_search('name', $header, true);
	$famCol = array_search('family', $header, true);
	if ($nameCol === false || $famCol === false) {
		fwrite(STDERR, "$path: no name or family column\n");
		exit(1);
	}
	$out = [];
	foreach ($lines as $line) {
		if ($line === '') {
			continue;
		}
		$f = explode("\t", $line);
		$out[$f[$nameCol]] = $f[$famCol] ?? '';
	}
	return $out;
}

function familyOf(array $families, string $name): ?string {
	$fam = $families[$name] ?? '';
	if ($fam === '' || $fam === '-') {
		return null;
	}
	return $fam;
}

function loadGrades(): array {
	$path = getenv('GRADES');
	if ($path === false || $path === '' || !is_file($path)) {
		return [[], 'GRADES is unset'];
	}
	$lines = file($path, FILE_IGNORE_NEW_LINES);
	if ($lines === false) {
		return [[], "$path: unreadable"];
	}
	$out = [];
	foreach ($lines as $line) {
		if ($line === '') {
			continue;
		}
		$f = explode("\t", $line);
		if (count($f) < 4 || $f[0] === '') {
			return [[], "$path: bad line"];
		}
		$out[$f[0]] = [$f[1], $f[2], $f[3]];
	}
	return [$out, ''];
}

function joined(string $name, array $vehicles, array $families, array $grades): ?array {
	$fam = familyOf($families, $name);
	if ($fam === null || !isset($vehicles[$name])) {
		return null;
	}
	$v = $vehicles[$name];
	$g = $grades[$name] ?? ['', '', ''];
	return [$fam, $v['engine'], $v['code'], $v['vehicle'], $v['year'], $v['gearbox'], $v['market'], $g[0], $g[1], $g[2]];
}

// percent is a hit/total cell as a whole percent, "-", or blank.
function percent(string $frac): string {
	if ($frac === '-') {
		return '-';
	}
	if (!preg_match('/^(\d+)\/(\d+)$/', $frac, $m) || $m[2] === '0') {
		return '';
	}
	return round(100 * $m[1] / $m[2]).'%';
}

function prettyJson(array $rows): string {
	$json = json_encode($rows, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE);
	if ($json === false) {
		fwrite(STDERR, "vehicles.json: encode failed\n");
		exit(1);
	}
	$json = preg_replace_callback('/^(    )+/m', function (array $m): string {
		return str_repeat('  ', intdiv(strlen($m[0]), 4));
	}, $json);
	return $json."\n";
}

function note(string $name, array $notes): array {
	if (isset($notes[$name])) {
		return $notes[$name];
	}
	$part = preg_replace('/-[^-]+$/', '', $name);
	$part = preg_replace('/[A-Z]+$/', '', $part);
	return array_pad($notes[$part] ?? [], 10, '');
}

function maps(string $path, string $name): ?int {
	$zip = new ZipArchive();
	if ($zip->open($path) !== true) {
		return null;
	}
	$json = $zip->getFromName($name.'.json');
	$zip->close();
	if ($json === false) {
		return null;
	}
	return substr_count($json, '"shape":');
}

function yearStart(string $year): float {
	if (preg_match('/^\d+(?:\.\d+)?/', $year, $m)) {
		return (float)$m[0];
	}
	return INF;
}

$rows = [];
foreach ($zips as $path) {
	$file = basename($path);
	$stem = substr($file, 0, -4);
	$date = substr($stem, strrpos($stem, '-') + 1);
	$name = substr($stem, 0, -strlen($date) - 1);
	$rows[] = [$file, $name, $date, note($name, $notes), maps($path, $name)];
}
usort($rows, function (array $a, array $b): int {
	$va = $a[3][3];
	$vb = $b[3][3];
	if (($va === '') !== ($vb === '')) {
		return ($va === '') <=> ($vb === '');
	}
	$c = strcasecmp($va, $vb);
	if ($c !== 0) {
		return $c;
	}
	$ya = yearStart($a[3][4]);
	$yb = yearStart($b[3][4]);
	if ($ya !== $yb) {
		return $ya <=> $yb;
	}
	$c = strcmp($a[3][4], $b[3][4]);
	if ($c !== 0) {
		return $c;
	}
	$c = strcmp($a[3][2], $b[3][2]);
	return $c !== 0 ? $c : strcmp($a[1], $b[1]);
});
?>
<!DOCTYPE html>
<meta charset="utf-8">
<title>Map definitions</title>
<style>
body { font: 16px/1.45 system-ui, sans-serif; max-width: 76rem; margin: 2rem auto; padding: 0 1rem; }
li { margin: 0.35rem 0; }
code { font-size: 0.95em; }
table { border-collapse: collapse; width: 100%; }
th, td { text-align: left; padding: 0.3rem 0.7rem 0.3rem 0; vertical-align: baseline; white-space: nowrap; }
thead th { font-weight: 600; border-bottom: 1px solid #ccc; }
thead button { font: inherit; font-weight: 600; background: none; border: 0; padding: 0; color: inherit; cursor: pointer; }
thead .num button { display: block; width: 100%; text-align: right; }
tr.group th { border-top: 1px solid #ccc; border-bottom: 0; padding-top: 0.9rem; font-weight: 600; }
tr.group:first-child th { border-top: 0; padding-top: 0.6rem; }
.num { text-align: right; font-variant-numeric: tabular-nums; }
</style>
<p><a href="../">Parent directory</a></p>
<h1>Map definitions for Bosch ME7 and related ECUs</h1>
<?php
$cols = [
	['image', 'Image', false],
	['ecu', 'ECU', false],
	['engine', 'Engine', false],
	['code', 'Code', false],
	['year', 'Year', false],
	['gearbox', 'Gearbox', false],
	['market', 'Market', false],
	['stage', 'Stage', false],
	['source', 'Source', false],
	['confidence', 'Confidence', true],
	['maps', 'Maps', true],
	['date', 'Last modified', true],
];
?>
<table>
<thead>
<tr>
<?php foreach ($cols as [$key, $label, $num]): ?>
<th<?= $num ? ' class="num"' : '' ?> aria-sort="none"><button type="button" data-key="<?= $key ?>"><?= htmlspecialchars($label) ?></button></th>
<?php endforeach ?>
</tr>
</thead>
<tbody id="defs">
<?php $group = null; ?>
<?php foreach ($rows as [$file, $name, $date, $cells, $count]): ?>
<?php $cells = array_pad($cells, 10, ''); ?>
<?php if ($cells[3] !== $group): $group = $cells[3]; ?>
<tr class="group"><th colspan="<?= count($cols) ?>"><?= htmlspecialchars($group) ?></th></tr>
<?php endif ?>
<tr data-image="<?= htmlspecialchars($name) ?>" data-ecu="<?= htmlspecialchars($cells[0]) ?>" data-engine="<?= htmlspecialchars($cells[1]) ?>" data-code="<?= htmlspecialchars($cells[2]) ?>" data-vehicle="<?= htmlspecialchars($cells[3]) ?>" data-year="<?= htmlspecialchars($cells[4]) ?>" data-gearbox="<?= htmlspecialchars($cells[5]) ?>" data-market="<?= htmlspecialchars($cells[6]) ?>" data-stage="<?= htmlspecialchars($cells[7]) ?>" data-confidence="<?= htmlspecialchars($cells[8]) ?>" data-source="<?= htmlspecialchars($cells[9]) ?>" data-maps="<?= $count === null ? '' : (string)$count ?>" data-date="<?= htmlspecialchars($date) ?>">
<td><a href="<?= htmlspecialchars($file) ?>"><code><?= htmlspecialchars($name) ?></code></a></td>
<?php foreach ([0, 1, 2, 4, 5, 6, 7] as $i): ?>
<td><?= htmlspecialchars($cells[$i]) ?></td>
<?php endforeach ?>
<td><?= htmlspecialchars($cells[9]) ?></td>
<td class="num"><?= htmlspecialchars(percent($cells[8])) ?></td>
<td class="num"><?= $count === null ? '' : number_format($count) ?></td>
<td class="num"><?= htmlspecialchars($date) ?></td>
</tr>
<?php endforeach ?>
</tbody>
</table>
<script>
(function () {
	const tbody = document.getElementById('defs');
	const heads = document.querySelectorAll('thead button');
	const startDesc = {confidence: true, maps: true, date: true};
	let key = '';
	let desc = false;

	function val(row, k) {
		return row.dataset[k] || '';
	}

	function cmpText(a, b) {
		return a.localeCompare(b, undefined, {sensitivity: 'base', numeric: true});
	}

	function cmpPrimary(a, b, k) {
		if (k === 'year') {
			const year = function (s) {
				const m = /^(\d+(?:\.\d+)?)/.exec(s);
				return m ? [parseFloat(m[1]), s] : [Infinity, s];
			};
			const pa = year(val(a, 'year'));
			const pb = year(val(b, 'year'));
			if (pa[0] !== pb[0]) {
				return pa[0] < pb[0] ? -1 : 1;
			}
			return cmpText(pa[1], pb[1]);
		}
		if (k === 'vehicle') {
			const va = val(a, 'vehicle');
			const vb = val(b, 'vehicle');
			if ((va === '') !== (vb === '')) {
				return va === '' ? 1 : -1;
			}
			return cmpText(va, vb);
		}
		if (k === 'stage' || k === 'source') {
			const order = {
				stage: {'1': 1, '2': 2, '3-': 3, '3': 4, '3+': 5, 'S+': 6},
				source: {verified: 1, DAMOS: 2, matched: 3, manual: 4, auto: 5},
			}[k];
			return (order[val(a, k)] || 0) - (order[val(b, k)] || 0);
		}
		if (k === 'confidence') {
			const ratio = function (s) {
				const m = /^(\d+)\/(\d+)$/.exec(s);
				if (!m) {
					return [0, 0];
				}
				const hit = Number(m[1]);
				const total = Number(m[2]);
				return [total === 0 ? 0 : hit / total, hit];
			};
			const pa = ratio(val(a, 'confidence'));
			const pb = ratio(val(b, 'confidence'));
			if (pa[0] !== pb[0]) {
				return pa[0] < pb[0] ? -1 : 1;
			}
			if (pa[1] !== pb[1]) {
				return pa[1] < pb[1] ? -1 : 1;
			}
			return 0;
		}
		if (k === 'maps' || k === 'date') {
			const num = function (row) {
				const s = val(row, k);
				return s === '' ? -Infinity : Number(s);
			};
			const na = num(a);
			const nb = num(b);
			if (na !== nb) {
				return na < nb ? -1 : 1;
			}
			return 0;
		}
		return cmpText(val(a, k), val(b, k));
	}

	function tie(a, b) {
		for (const k of ['vehicle', 'year', 'code', 'image']) {
			const c = cmpPrimary(a, b, k);
			if (c) {
				return c;
			}
		}
		return 0;
	}

	function rebuild() {
		const rows = [...tbody.querySelectorAll('tr[data-image]')];
		rows.sort(function (a, b) {
			const g = cmpPrimary(a, b, 'vehicle');
			if (g) {
				return g;
			}
			if (key === '') {
				return tie(a, b);
			}
			if (key === 'stage' || key === 'confidence' || key === 'source') {
				const ea = val(a, key) === '';
				const eb = val(b, key) === '';
				if (ea !== eb) {
					return ea ? 1 : -1;
				}
			}
			const c = cmpPrimary(a, b, key);
			if (c) {
				return desc ? -c : c;
			}
			return tie(a, b);
		});
		tbody.replaceChildren();
		let prev = null;
		for (const row of rows) {
			const v = val(row, 'vehicle');
			if (v !== prev) {
				const tr = document.createElement('tr');
				tr.className = 'group';
				const th = document.createElement('th');
				th.colSpan = heads.length;
				th.textContent = v;
				tr.append(th);
				tbody.append(tr);
				prev = v;
			}
			tbody.append(row);
		}
		for (const btn of heads) {
			const on = btn.dataset.key === key;
			btn.closest('th').setAttribute('aria-sort', on ? (desc ? 'descending' : 'ascending') : 'none');
		}
	}

	for (const btn of heads) {
		btn.addEventListener('click', function () {
			const k = btn.dataset.key;
			if (k === key) {
				desc = !desc;
			} else {
				key = k;
				desc = !!startDesc[k];
			}
			rebuild();
		});
	}
})();
</script>
<h2>Stage</h2>
<ul>
<li>1, 2, 3-, 3, then 3+. An open stage 1 is <code>-</code>. Blank when the image is not in the locate set (for example M3.8 / M5.9). <code>-</code> for <code>DAMOS</code>.</li>
</ul>
<h2>Source</h2>
<ul>
<li><code>verified</code> - A person checked every definition against the image.</li>
<li><code>DAMOS</code> - From the Bosch DAMOS or A2L. Canonical.</li>
<li><code>matched</code> - Found by program, and every name in the reference it replaced is at the same address, apart from known exceptions such as <code>ESKONF</code>.</li>
<li><code>manual</code> - Made by hand, and no one has verified it.</li>
<li><code>auto</code> - Found by program, with no reference or one that only partly agrees.</li>
</ul>
<h2>Confidence</h2>
<ul>
<li>How many located tuner bodies hold up (monotonic axes, peer agreement). Shown for <code>auto</code>, <code>matched</code>, <code>manual</code>, and <code>verified</code>.</li>
<li><code>-</code> for <code>DAMOS</code>: that file is the oracle; stage and confidence are not locate scores for it.</li>
</ul>
<h2>Inside a zip</h2>
<ul>
<li><code>.json</code> - Full canonical definition. All other formats are generated from it.</li>
<li><code>.kp</code> - WinOLS map pack.</li>
<li><code>.xdf</code> - Full TunerPro definition. TunerPro may not load a large full XDF, consider using the "tuner" subset.</li>
<li><code>.meta.json</code> - Data the XDF cannot hold. Kept next its XDF.</li>
<li><code>.csv</code> - Simple text map list.</li>
<li><code>-tuner.xdf</code>, <code>-tuner.meta.json</code>, <code>-tuner.csv</code> - ME7 only: the tuning maps, filed by category. Open this XDF in TunerPro.</li>
</ul>
<p>Generated by <a href="https://github.com/nyetlabs/me7-logger">me7-logger</a> and <a href="https://github.com/nyetlabs/xdfkit">xdfkit</a>. It may change or disappear without notice. No warranty: check every map against your image before writing anything to an ECU.</p>
