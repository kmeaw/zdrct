'use strict';

window.addEventListener('DOMContentLoaded', (event) => {
	console.log("DOMContentLoaded!");

	document.querySelectorAll(".saveme").forEach((el) => {
		if (el.value === "" && localStorage.hasOwnProperty(el.dataset.name)) {
			el.value = localStorage[el.dataset.name];
		}
	})

	document.querySelectorAll(".saveme").forEach((el) => {
		if (localStorage.hasOwnProperty(el.dataset.name) && el.dataset.autosubmit) {
			el.form.submit();
			return;
		}
	})

	window.addEventListener('beforeunload', (event) => {
		console.log("beforeunload triggered");
		document.querySelectorAll(".saveme").forEach((el) => {
			if (el.value !== "") {
				localStorage[el.dataset.name] = el.value;
			}
		});
	})
});

// vim: ai:ts=8:sw=8:noet:syntax=js
