'use strict';

window.addEventListener('DOMContentLoaded', (event) => {
	const $script = document.getElementById('script');
	const $scriptform = document.getElementById('scriptform');
	const $scriptmsg = document.getElementById('scriptmsg');

	const $submit = $scriptform.querySelector('input[type=submit]');
	const $rewards_table = document.getElementById('rewards_table');

	const $iwad = document.getElementById('wizard_iwad');
	const $patch = document.getElementById('wizard_image');
	const $wizard_rescale = document.getElementById('wizard_rescale');
	const $sound = document.getElementById('wizard_sound');
	const $canvas = document.getElementById('wizard_lump_canvas');
	const $audio = document.getElementById('wizard_lump_audio');
	const $wizard_msg = document.getElementById('wizard_msg');
	const $wizard_generate = document.getElementById('wizard_generate');
	const $wizard_id = document.getElementById('wizard_id');
	const $wizard_name = document.getElementById('wizard_name');
	const $wizard_reply = document.getElementById('wizard_reply');

	const $bwizard_id = document.getElementById('buttonwizard_id');
	const $bwizard_name = document.getElementById('buttonwizard_name');
	const $bwizard_image = document.getElementById('buttonwizard_image');
	const $bwizard_price = document.getElementById('buttonwizard_price');
	const $bwizard_code = document.getElementById('buttonwizard_code');
	const $bwizard_generate = document.getElementById('buttonwizard_generate');

	const reader = new FileReader();

	$rewards_table.querySelectorAll('form.delete-form').forEach((frm) => frm.addEventListener('submit', (event) => {
		event.preventDefault();
		event.stopPropagation();

		const req = fetch(frm.action + '?xhr=1', {
			method: frm.method,
			mode: 'same-origin',
			cache: 'no-cache',
			credentials: 'omit',
			headers: {
				'Content-Type': 'application/x-www-form-urlencoded',
				'Accept': 'application/json'
			},
			redirect: 'error',
		});

		req
			.then((resp) => resp.json())
			.then((json) => {
				if (json.ok) {
					frm.parentNode.parentNode.parentNode.removeChild(frm.parentNode.parentNode);
					return true;
				}

				frm.innerText = json.description || json.error;
			})
			.catch((err) => frm.innerText = err);

		return false;
	}));

	$submit.disabled = true;

	const cm = CodeMirror.fromTextArea($script,
	{
		mode:        'go',
		lineNumbers: false
	});

	$scriptform.addEventListener('submit', (event) => {
		event.preventDefault();
		event.stopPropagation();

		const req = fetch($scriptform.action + '?xhr=1', {
			method: 'POST',
			mode: 'same-origin',
			cache: 'no-cache',
			credentials: 'omit',
			headers: {
				'Content-Type': 'application/x-www-form-urlencoded',
				'Accept': 'application/json'
			},
			redirect: 'error',
			body: new URLSearchParams({
				script: $script.value
			}).toString()
		});

		req
			.then((resp) => resp.json())
			.then((json) => {
				cm.getDoc().getAllMarks().forEach((mark) => mark.clear());
				if (json.line && json.column) {
					cm.focus();
					cm.setCursor({
						line: json.line - 1,
						ch: json.column - 1
					});
					cm.getDoc().markText({
						line: json.line - 1,
						ch: json.column - 1,
					}, {
						line: json.line - 1,
						ch: json.column
					}, {
						css: 'background-color: red'
					});
				}

				if (json.error) {
					$scriptmsg.innerText = json.description || json.error;
				} else {
					location.search = '?tab=script';
				}
			});

		return false;
	});

	$submit.disabled = false;

	$iwad.addEventListener('change', (event) => {
		if ($iwad.files.length !== 1) {
			return;
		}

		reader.readAsArrayBuffer($iwad.files[0]);
	});

	let iwad;

	const load_wad = (arrayBuffer) => {
		try {
			iwad = new IWAD(arrayBuffer);
		} catch(err) {
			$wizard_msg.innerText = 'ERROR: ' + err;
			return false;
		}

		$patch.disabled = true;
		$patch.innerHTML = '';

		$sound.disabled = true;
		$sound.innerHTML = '';

		const $option_none = document.createElement('option');
		$option_none.value = '';
		$option_none.text = '(none)';
		$patch.add($option_none);
		$sound.add($option_none.cloneNode(true));

		Object.keys(iwad.patches).sort().forEach((patch) => {
			const $option = document.createElement('option');
			$option.value = patch;
			$option.text = patch;
			$patch.add($option);
		});

		$patch.disabled = false;
		const redraw = (event) => {
			const value = $patch.options[$patch.selectedIndex].value;
			if (value && iwad.patches[value]) {
				const size = $wizard_rescale.options[$wizard_rescale.selectedIndex].value;
				iwad.patches[value].render($canvas, parseInt(size));
			} else {
				const ctx = $canvas.getContext('2d');
				ctx.clearRect(0, 0, $canvas.width, $canvas.height);
				$canvas.width = 0;
				$canvas.height = 0;
			}
		};
		$patch.addEventListener('change', redraw)
		$wizard_rescale.addEventListener('change', redraw)

		Object.keys(iwad.sounds).sort().forEach((patch) => {
			const $option = document.createElement('option');
			$option.value = patch;
			$option.text = patch;
			$sound.add($option);
		});

		$sound.disabled = false;
		$sound.addEventListener('change', (event) => {
			const value = $sound.options[$sound.selectedIndex].value;
			if (value) {
				iwad.sounds[value].render($audio);
			} else {
				$audio.src = null;
			}
		});
		$wizard_msg.innerText = '';
	};

	reader.addEventListener('load', (event) => {
		$wizard_msg.innerText = 'Loading IWAD, please wait...';
		const arrayBuffer = event.target.result;
		setTimeout(load_wad, 100, arrayBuffer);
	});

	if ($iwad.files.length === 1) {
		reader.readAsArrayBuffer($iwad.files[0]);
	}

	$wizard_generate.addEventListener('click', (event) => {
		event.stopPropagation();
		event.preventDefault();

		const id = $wizard_id.value.toLowerCase().replace(/[ -]/g, '_');
		const camel = id.substr(0, 1).toUpperCase() + id.substr(1);

		const promises = [];
		let code = cm.getValue();
		code = code + `
${camel} = new(Actor)
${camel}.ID = ${JSON.stringify($wizard_id.value)}
${camel}.Name = ${JSON.stringify($wizard_name.value)}
`;

		if ($canvas.width > 0 && $canvas.height > 0) {
			const blobp = new Promise((resolve, reject) => {
				$canvas.toBlob((blob) => resolve(blob), 'image/png');
			});

			const promise = blobp.then((blob) =>
				fetch('/upload/assets/' + encodeURIComponent(id) + '.png', {
					method: 'POST',
					mode: 'same-origin',
					cache: 'no-cache',
					credentials: 'omit',
					redirect: 'error',
					body: blob
				})
				.then((resp) => resp.json())
				.then((json) => {
					if (json.error) throw json.error;
				})
				.then(() => {
					code = code + `${camel}.AlertImage = '${encodeURIComponent(id)}.png'\n`;
				})
			);

			promises.push(promise);
		}

		const audio_value = $sound.options[$sound.selectedIndex].value;
		if (audio_value) {
			const wav = iwad.sounds[audio_value].toWav();
			const req = fetch('/upload/assets/' + encodeURIComponent(id) + '.wav', {
				method: 'POST',
				mode: 'same-origin',
				cache: 'no-cache',
				credentials: 'omit',
				redirect: 'error',
				body: wav
			});

			const promise = req
				.then((resp) => resp.json())
				.then((json) => {
					if (json.error) throw json.error;
				})
				.then(() => {
					code = code + `${camel}.AlertSound = '${encodeURIComponent(id)}.wav'\n`;
				});

			promises.push(promise);
		}

		if ($wizard_reply.value) {
			code = code + `${camel}.Reply = ${JSON.stringify($wizard_reply.value)}\n`;
		}

		Promise.all(promises)
			.then(() => {
				cm.setValue(code);

				$bwizard_id.value = id;
				$bwizard_name.value = $wizard_name.value;
				$bwizard_code.value = 'return spawn_actor(' + camel + ')';
				$bwizard_image.value = encodeURIComponent(id) + '.png';
				document.getElementById('nav-buttonwizard-tab').click();
				$bwizard_price.focus();
			})
			.catch((err) => {
				$wizard_msg.innerText = 'ERROR: ' + err;
				$wizard_msg.scrollIntoView();
			});
	});

	$bwizard_generate.addEventListener('click', (event) => {
		event.stopPropagation();
		event.preventDefault();

		let new_code = `cmd_${$bwizard_id.value} = `;
		if ($bwizard_price.value > 0) {
			new_code = new_code + `redeem(${$bwizard_price.value}, func() {`;
		} else {
			new_code = new_code + 'func() {';
		}
		new_code = new_code + '\n' + $bwizard_code.value.split('\n').map((line) => `  ${line}\n`);
		if ($bwizard_price.value > 0) {
			new_code = new_code + '})\n';
		} else {
			new_code = new_code + '}\n';
		}

		new_code = new_code + `
button_${$bwizard_id.value} = new(Command)
button_${$bwizard_id.value}.Cmd = ${JSON.stringify($bwizard_id.value)}
button_${$bwizard_id.value}.Text = ${JSON.stringify($bwizard_name.value)}
button_${$bwizard_id.value}.Image = ${JSON.stringify($bwizard_image.value)}
add_command(button_${$bwizard_id.value})
`;

		if ($bwizard_price.value > 0) {
			new_code = new_code + `
reward_${$bwizard_id.value} = new(Reward)
reward_${$bwizard_id.value}.Cost = ${$bwizard_price.value}
reward_${$bwizard_id.value}.Title = ${JSON.stringify($wizard_name.value)}
reward_${$bwizard_id.value}.IsEnabled = true
map_reward(reward_${$bwizard_id.value}, button_${$bwizard_id.value})
`;
		}

		cm.setValue(cm.getValue() + '\n' + new_code);
		document.getElementById('nav-script-tab').click();
	});

	setInterval(() => {
		fetch('/check_csrf?csrf=' + encodeURIComponent(csrf))
			.then((resp) => resp.json())
			.then((j) => {
				if (!j.valid) window.close();
			});
	}, 2000);
});

// vim: ai:ts=8:sw=8:noet:syntax=js
