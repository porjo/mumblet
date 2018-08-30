
var loc = window.location, ws_uri;
if (loc.protocol === "https:") {
	ws_uri = "wss:";
} else {
	ws_uri = "ws:";
}
ws_uri += "//" + loc.host;
var path = loc.pathname.replace(/\/$/, '');
ws_uri += path + "/websocket";

var ws = new WebSocket(ws_uri);

var pc;
var sd_uri = loc.protocol + "//" + loc.host + path + "/sdp";

$(function(){

	$("#advanced-toggle").click(function() {
		$(this).find(".text").toggleClass('hidden');
		$("#advanced-form").slideToggle();
	});

	$("#connect-button").click(function() {
		if (ws.readyState === 1) {
			$("#output").show();
			var params = {};
			params.Url = $("#url").val();
			params.Hostname = $("#hostname").val();
			params.Port = Number($("#port").val());
			params.Username = $("#username").val();
			params.Channel = $("#channel").val();
			params.SessionDescription = pc.localDescription.sdp;
			var val = {Key: 'connect', Value: params};
			ws.send(JSON.stringify(val));
			log("js: Connecting to host");
		} else {
			log("WS socket not ready");
		}
	});

	/*
	ws.onopen = function() {
	};
	*/

	ws.onmessage = function (e)	{
		var msg = JSON.parse(e.data);
		if( 'Key' in msg ) {
			switch (msg.Key) {
				case 'info':
					log("Info: " + msg.Value);
					break;
				case 'error':
					log("Error: " + msg.Value);
					break;
				case 'sd_answer':
					connectRTC(msg.Value);
					break;
			}
		}
	};

	ws.onclose = function()	{
		log("WS connection closed");
	};



	//
	// -------- WebRTC ------------
	//

	pc = new RTCPeerConnection({
		iceServers: [
			{
				urls: "stun:stun.l.google.com:19302"
			}
		]
	})
	var log = msg => {
		$("#status").append(msg + '\n');
	}

	pc.ontrack = function (event) {
		var el = document.createElement(event.track.kind)
		el.srcObject = event.streams[0]
		el.autoplay = true
		el.controls = true

		$("#output-media").append(el);
	}

	pc.oniceconnectionstatechange = e => log("js: rtc state change, " + pc.iceConnectionState)
	pc.onicecandidate = event => {
		if (event.candidate === null) {
			//document.getElementById('localSessionDescription').value = btoa(pc.localDescription.sdp)
		}
	}

	pc.createOffer({
	//	offerToReceiveVideo: true, 
		offerToReceiveAudio: true
	}).then(d => pc.setLocalDescription(d)).catch(log)

	function connectRTC(sd) {
			if (sd === '') {
				return alert('Session Description must not be empty')
			}

			try {
				pc.setRemoteDescription(new RTCSessionDescription({type: 'answer', sdp: sd}))
			} catch (e) {
				alert(e)
			}
	}

});
