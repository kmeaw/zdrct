'use strict';

window.addEventListener('DOMContentLoaded', (event) => {
  const $message = document.getElementById('message');

  const usp = new URLSearchParams(location.hash.substr(1));
  fetch('/oauth', {
    method: 'POST',
    mode: 'same-origin',
    cache: 'no-cache',
    credentials: 'omit',
    headers: {
      'Content-Type': 'application/json'
    },
    redirect: 'error',
    body: JSON.stringify(Object.fromEntries(usp.entries()))
  })
    .then((r) => {
      if (!r.ok) {
	return r.text().then((t) => { throw t; });
      }

      return r.json();
    })
    .then((result) => {
      if (!result.ok) {
	throw 'Bad result: ' + JSON.stringify(result);
      }

      location.replace('/');
    })
    .catch((e) => $message.innerText = 'An error has occurred: ' + e);
});
