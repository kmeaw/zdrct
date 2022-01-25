'use strict';

window.addEventListener('DOMContentLoaded', (event) => {
	const $script = document.getElementById('script');
	const $scriptform = document.getElementById('scriptform');
	const $scriptmsg = document.getElementById('scriptmsg');

	const $submit = $scriptform.querySelector('input[type=submit]');
	const $rewards_table = document.getElementById('rewards_table');

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

	setInterval(() => {
		fetch('/check_csrf?csrf=' + encodeURIComponent(csrf))
			.then((resp) => resp.json())
			.then((j) => {
				if (!j.valid) window.close();
			});
	}, 2000);
});

// vim: ai:ts=8:sw=8:noet:syntax=js
