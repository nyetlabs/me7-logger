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
		$joined = [];
		foreach ($vehicles as $name => $_) {
			$cells = joined($name, $vehicles, $families);
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

function joined(string $name, array $vehicles, array $families): ?array {
	$fam = familyOf($families, $name);
	if ($fam === null || !isset($vehicles[$name])) {
		return null;
	}
	$v = $vehicles[$name];
	return [$fam, $v['engine'], $v['code'], $v['vehicle'], $v['year'], $v['gearbox'], $v['market']];
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
	return $notes[$part] ?? ['', '', '', '', '', '', ''];
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
	$fa = str_starts_with($a[3][0], 'ME') ? 1 : 0;
	$fb = str_starts_with($b[3][0], 'ME') ? 1 : 0;
	if ($fa !== $fb) {
		return $fa <=> $fb;
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
	$c = strcmp($a[3][3], $b[3][3]);
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
th { font-weight: 600; border-bottom: 1px solid #ccc; }
.num { text-align: right; font-variant-numeric: tabular-nums; }
</style>
<p><a href="../">Parent directory</a></p>
<h1>Map definitions for Bosch ME7 and related ECUs</h1>
<table>
<tr><th>Image</th><th>ECU</th><th>Engine</th><th>Code</th><th>Vehicle</th><th>Year</th><th>Gearbox</th><th>Market</th><th class="num">Maps</th><th class="num">Last modified</th></tr>
<?php foreach ($rows as [$file, $name, $date, $cells, $count]): ?>
<tr>
<td><a href="<?= htmlspecialchars($file) ?>"><code><?= htmlspecialchars($name) ?></code></a></td>
<?php foreach ($cells as $cell): ?>
<td><?= htmlspecialchars($cell) ?></td>
<?php endforeach ?>
<td class="num"><?= $count === null ? '' : number_format($count) ?></td>
<td class="num"><?= htmlspecialchars($date) ?></td>
</tr>
<?php endforeach ?>
</table>
<h2>Inside a zip</h2>
<ul>
<li><code>.json</code> - Full canonical definition. All other formats are generated from it.</li>
<li><code>.kp</code> - WinOLS map pack.</li>
<li><code>.xdf</code> - Full TunerPro definition. TunerPro may not load a large full XDF, consider using the "tuner" subset.</li>
<li><code>.meta.json</code> - Data the XDF cannot hold. Kept next its XDF.</li>
<li><code>.csv</code> - Simple text map list.</li>
<li><code>-tuner.xdf</code>, <code>-tuner.meta.json</code>, <code>-tuner.csv</code> - ME7 only: the tuning maps, filed by category. Open this XDF in TunerPro.</li>
</ul>
<p>Generated by <a href="https://github.com/nyetlabs/xdfkit">xdfkit</a>. It may change or disappear without notice. No warranty: check every map against your image before writing anything to an ECU.</p>
