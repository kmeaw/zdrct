'use strict';

window.addEventListener('DOMContentLoaded', (event) => {
	let conn = new WebSocket('ws://' + location.host + '/alerts/ws');
	const $connecting = document.getElementById('connecting');
	const $msg = document.getElementById('msg');
	const $img = document.getElementById('img');
	const $text = document.getElementById('text');
	let fader = null;

	conn.addEventListener('open', (event) => {
		$connecting.style.display = 'none';
	});

	// Listen for messages
	conn.addEventListener('message', (event) => {
		const data = JSON.parse(event.data);
		$msg.classList.remove('fade');
		$text.innerText = data.text;
		if (data.image) {
			$img.setAttribute('src', data.image);
			$img.style.display = 'block';
		} else {
			$img.style.display = 'none';
		}
		if (fader) {
			clearTimeout(fader);
		}
		fader = setTimeout(() => {
			$msg.classList.add('fade');
			fader = null;
		}, 5000);
	});	

	conn.addEventListener('close', (event) => {
		setTimeout(() => {
			conn = new WebSocket('ws://' + location.host + '/alerts/ws');
		}, 5000);
	});	

	conn.addEventListener('error', (event) => {
		console.error(event);
		setTimeout(() => {
			conn = new WebSocket('ws://' + location.host + '/alerts/ws');
		}, 5000);
	});	
});
