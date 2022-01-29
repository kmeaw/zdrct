'use strict';

class Sound {
	constructor(iwad, name) {
		this.name = name;
		this.iwad = iwad;
		this.arrayBuffer = iwad.read_lump(name);

		[this.format, this.rate] = new Uint16Array(this.arrayBuffer.slice(0, 4));
		[this.nsamples] = new Uint32Array(this.arrayBuffer.slice(4, 8));
		this.samples = new Uint8Array(this.arrayBuffer.slice(0x18, 0x18 + this.nsamples));
	}

	toWav() {
		const ascii = new TextEncoder('ascii');
		const wav = new Uint8Array(44 + this.samples.length);
		const dataview = new DataView(wav.buffer);
		wav.set(ascii.encode('RIFF'), 0);
		dataview.setUint32(4, 36 + this.samples.length, true);
		wav.set(ascii.encode('WAVEfmt '), 8);
		dataview.setUint32(16, 16, true);
		dataview.setUint16(20, 1, true);
		dataview.setUint16(22, 1, true);
		dataview.setUint32(24, this.rate, true);
		dataview.setUint32(28, this.rate, true);
		dataview.setUint16(32, 1, true);
		dataview.setUint16(34, 8, true);
		wav.set(ascii.encode('data'), 36);
		dataview.setUint32(40, this.samples.length * 2, true);
		wav.set(this.samples, 44);

		return new Blob([wav], {type: 'audio/wav'});
	}

	render($audio) {
		$audio.src = URL.createObjectURL(this.toWav());
	}
}

class Patch {
	constructor(iwad, name) {
		this.name = name;
		this.iwad = iwad;
		this.arrayBuffer = iwad.read_lump(name);
		[this.width, this.height, this.leftoffset, this.topoffset] = new Uint16Array(this.arrayBuffer.slice(0, 8));
		this.columns = new Uint32Array(this.arrayBuffer.slice(8, 8 + 4 * this.width));
		const data = new Uint8Array(this.width * this.height * 4);
		this.columns.forEach((offset, ic) => {
			const column = this.arrayBuffer.slice(offset);
			for (let i = 0; i + 4 < column.byteLength;) {
				const [topdelta, length] = new Uint8Array(column.slice(i, i + 2));
				if (topdelta == 0xFF) break;
				i += 3; // skip unused byte
				const pixels = new Uint8Array(column.slice(i, i + length));
				pixels.forEach((v, j) => {
					j += topdelta;
					data[(ic + j * this.width) * 4 + 0] = iwad.playpal[v * 3 + 0];
					data[(ic + j * this.width) * 4 + 1] = iwad.playpal[v * 3 + 1];
					data[(ic + j * this.width) * 4 + 2] = iwad.playpal[v * 3 + 2];
					data[(ic + j * this.width) * 4 + 3] = 255;
				});
				i += length + 1;
			}
		});
		this.imageData = new ImageData(new Uint8ClampedArray(data.buffer), this.width, this.height);
	}

	render($canvas, size) {
		let dx = 0, dy = 0, dw = this.width, dh = this.height;
		if (size) {
			$canvas.width = size;
			$canvas.height = size;
			const r = size / Math.min(dw, dh);
			dw *= r;
			dh *= r;
			dx = size / 2 - dw / 2;
			dy = size / 2 - dh / 2;
		} else {
			$canvas.width = this.width;
			$canvas.height = this.height;
		}
		const context = $canvas.getContext('2d');
		createImageBitmap(this.imageData).then((renderer) => {
			context.drawImage(renderer, dx, dy, dw, dh);
		});
	}
}

class IWAD {
	constructor(arrayBuffer) {
		this.arrayBuffer = arrayBuffer;
		this.files = {};
		this.patches = {};
		this.sounds = {};

		const ascii = new TextDecoder('ascii');
		const sig4 = ascii.decode(new Uint8Array(arrayBuffer, 0, 4));
		const [lumps, dir_offset] = new Uint32Array(arrayBuffer, 4, 8);
		if (sig4 != 'IWAD') {
			throw 'Bad input file header: ' + sig4;
		}

		if (dir_offset + 16 > arrayBuffer.byteLength) {
			throw 'Bad info table offset: ' + dir_offset;
		}

		let is_patch = false;
		let idx = 0;
		const patchlist = [];

		for (let offset = dir_offset; offset + 16 < arrayBuffer.byteLength; offset += 16) {
			const [pos, size] = new Uint32Array(arrayBuffer, offset, 8);
			let name = ascii.decode(new Uint8Array(arrayBuffer, offset + 8, 8));
			const namezidx = name.indexOf('\0');
			if (namezidx != -1) {
				name = name.substr(0, namezidx);
			}
			idx++;

			if (/^S\d?_START$/.exec(name)) {
				is_patch = true;
				continue;
			} else if (/^S\d?_END$/.exec(name)) {
				is_patch = false;
				continue;
			}
			this.files[name] = {
				pos: pos,
				size: size,
				index: idx,
			};
			if (is_patch) {
				patchlist.push(name);
			} else if (size > 8) {
				if (ascii.decode(new Uint8Array(arrayBuffer, pos, 2)) == 'DS') {
					this.sounds[name] = new Sound(this, name);
				} else {
					const [fmt, rate] = new Uint16Array(arrayBuffer, pos, 2);
					if (fmt == 3 && rate > 0 && (rate % 11025) == 0) {
						this.sounds[name] = new Sound(this, name);
					}
				}
			}
		}
		this.playpal = new Uint8Array(this.read_lump('PLAYPAL'));
		patchlist.forEach((name) => {
			this.patches[name] = new Patch(this, name);
		});
	}

	read_lump(name) {
		const file = this.files[name];
		if (!file) {
			throw 'File not found: ' + name;
		}
		if (file.pos + file.size > this.arrayBuffer.byteLength) {
			throw 'File ' + name + ' has invalid offset or position';
		}
		return this.arrayBuffer.slice(file.pos, file.pos + file.size);
	}
}
