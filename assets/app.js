'use strict';

window.addEventListener('DOMContentLoaded', (event) => {
	const $ = document.getElementById.bind(document);

	const $script = $('script');
	const $scriptform = $('scriptform');
	const $scriptmsg = $('scriptmsg');

	const $submit = $scriptform ? $scriptform.querySelector('input[type=submit]') : null;
	const $rewards_table = $('rewards_table');

	const $iwad = $('wizard_iwad');
	const $patch = $('wizard_image');
	const $wizard_rescale = $('wizard_rescale');
	const $sound = $('wizard_sound');
	const $canvas = $('wizard_lump_canvas');
	const $audio = $('wizard_lump_audio');
	const $wizard_msg = $('wizard_msg');
	const $wizard_generate = $('wizard_generate');
	const $wizard_id = $('wizard_id');
	const $wizard_name = $('wizard_name');
	const $wizard_reply = $('wizard_reply');

	const $bwizard_id = $('buttonwizard_id');
	const $bwizard_name = $('buttonwizard_name');
	const $bwizard_image = $('buttonwizard_image');
	const $bwizard_price = $('buttonwizard_price');
	const $bwizard_code = $('buttonwizard_code');
	const $bwizard_generate = $('buttonwizard_generate');

	const $table_editor = $('table-editor');
	const $table_form = $('table-form');
	const $table_name = $('table-name');

	const reader = new FileReader();

	document.querySelectorAll('.youtube-resolver').forEach((frm) => frm.addEventListener('submit', async (event) => {
		event.preventDefault();
		event.stopPropagation();

		const params = new URLSearchParams(new FormData(frm));

		const resp = await fetch(frm.action + '?' + params.toString(), {
			cache: 'no-cache',
			credentials: 'omit',
			headers: { 'Accept': 'application/json' }
		});
		if (!resp.ok) throw Error(`HTTP ${resp.status}`);
		const data = await resp.json();
		try {
			const item = data.result.items[0];
			if (item === undefined) {
				return;
			}
			if (item.kind === 'youtube#channel') {
				$('youtube_channel_id').value = item.id;
				$('youtube_channel_id').focus();
				navigator.sendBeacon('/youtube/channel',
					new URLSearchParams({
						csrf: csrf,
						channel_id: item.id,
					})
				);
			} else if (item.kind == 'youtube#searchResult' && item.id.kind === 'youtube#video') {
				$('youtube_video_id').value = item.id.videoId;
				$('youtube_video_id').focus();
			} else if (item.kind == 'youtube#video' && item.liveStreamingDetails && item.liveStreamingDetails.activeLiveChatId) {
				$('youtube_live_chat_id').value = item.liveStreamingDetails.activeLiveChatId;
				$('youtube_live_chat_id').focus();
			}
		} catch(e) {
			console.error(e);
		}
	}));

	if ($rewards_table)
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

				frm.textContent = json.description || json.error;
			})
			.catch((err) => frm.textContent = err);

		return false;
	}));

	if ($scriptform) {
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
				script: $script.value,
				csrf: csrf
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
					$scriptmsg.textContent = json.description || json.error;
				} else {
					location.search = '?tab=script';
				}
			});

		return false;
	});

	$submit.disabled = false;
	}

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
			$wizard_msg.textContent = 'ERROR: ' + err;
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
		$wizard_msg.textContent = '';
	};

	reader.addEventListener('load', (event) => {
		$wizard_msg.textContent = 'Loading IWAD, please wait...';
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
				$('nav-buttonwizard-tab').click();
				$bwizard_price.focus();
			})
			.catch((err) => {
				$wizard_msg.textContent = 'ERROR: ' + err;
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
		$('nav-script-tab').click();
	});

	$table_form.addEventListener('submit', async (event) => {
		event.stopPropagation();
		event.preventDefault();

		if ($table_name.value.length <= 1) {
			$table_name.focus();
			return false;
		}

		const params = new URLSearchParams(new FormData($table_form));

		try {
			const resp = await fetch($table_form.action + '?' + params.toString());
			if (!resp.ok) {
				throw new Error(`HTTP ${resp.status}`);
			}

			$table_editor.innerHTML = await resp.text();
			$table_editor.querySelectorAll('script').forEach($s0 => {
				const $s1 = document.createElement('script');
				Array.from($s0.attributes).forEach(a => {
					$s1.setAttribute(a.name, a.value);
				});
				$s1.textContent = $s0.textContent;
				$s0.parentNode.replaceChild($s1, $s0);
			});
		} catch(e) {
			$table_editor.textContent = 'ERROR: ' + e;
		}
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
