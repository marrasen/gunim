package gunim.android;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.media.AudioAttributes;
import android.media.AudioFocusRequest;
import android.media.AudioManager;
import android.media.MediaMetadata;
import android.media.session.MediaSession;
import android.media.session.PlaybackState;
import android.os.IBinder;

/**
 * GunimService shows what a gunim program plays in Android's media
 * controls, through a media session, on the lock screen, in quick
 * settings and in a media notification, and keeps the program running
 * while it plays in the background, as a foreground service. A media
 * notification needs no leave to notify. The controls' buttons go to Go
 * as Native.media; Go hands it what plays through Native.nowPlaying.
 *
 * It also holds the audio focus while the program plays, so other media
 * players stop when it starts, and it pauses when another one starts. It
 * pauses when the sound would move to the speaker, as when headphones are
 * unplugged.
 */
public class GunimService extends Service {
	private static final String CHANNEL = "gunim.playing";
	private static final int ID = 1;

	// The notification's buttons, as the actions of the intents they
	// send the service.
	private static final String PLAY = "gunim.play", PAUSE = "gunim.pause",
		NEXT = "gunim.next", PREVIOUS = "gunim.previous";

	/** State is what plays, as Go last told. */
	static final class State {
		final boolean playing;
		final String title, artist, album;
		final long length, position;
		final Bitmap art;

		State(boolean playing, String title, String artist, String album, long length, long position, byte[] art) {
			this.playing = playing;
			this.title = title;
			this.artist = artist;
			this.album = album;
			this.length = length;
			this.position = position;
			this.art = art != null ? BitmapFactory.decodeByteArray(art, 0, art.length) : null;
		}
	}

	// state is what plays; running is the service while it runs.
	private static State state;
	private static GunimService running;

	private MediaSession session;

	// focus is the audio focus request; held is true while the program
	// holds the focus. lent is true while a call or another short sound
	// has it and the program paused for it, to play again when it is
	// given back.
	private static AudioFocusRequest focus;
	private static boolean held, lent;

	// noisy pauses the program when Android is about to move its sound to
	// the speaker; it listens while the program plays.
	private static final BroadcastReceiver noisy = new BroadcastReceiver() {
		@Override
		public void onReceive(Context c, Intent i) {
			if (AudioManager.ACTION_AUDIO_BECOMING_NOISY.equals(i.getAction())) {
				Native.media(2, 0);
			}
		}
	};
	private static boolean listening;

	/**
	 * show shows st in the media controls, starting the service the
	 * first time something plays, or takes the controls away for null.
	 * It runs on the UI thread.
	 */
	static void show(Context app, State st) {
		state = st;
		if (app == null) {
			return;
		}
		audioFocus(app, st);
		listen(app, st != null && st.playing);
		if (st == null) {
			app.stopService(new Intent(app, GunimService.class));
			return;
		}
		if (running != null) {
			running.apply();
			return;
		}
		if (st.playing) {
			app.startForegroundService(new Intent(app, GunimService.class));
		}
	}

	/**
	 * audioFocus asks for the audio focus when st starts to play, and
	 * gives it up when st stops or the user pauses it. It keeps the focus
	 * through a pause it made for a call, to get it back when the call
	 * ends. It runs on the UI thread, as do the focus's changes.
	 */
	private static void audioFocus(Context app, State st) {
		AudioManager am = app.getSystemService(AudioManager.class);
		if (focus == null) {
			focus = new AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN)
				.setAudioAttributes(new AudioAttributes.Builder()
					.setUsage(AudioAttributes.USAGE_MEDIA)
					.setContentType(AudioAttributes.CONTENT_TYPE_MUSIC)
					.build())
				.setOnAudioFocusChangeListener(new Focus(app.getPackageName()))
				.build();
		}
		boolean playing = st != null && st.playing;
		if (playing) {
			if (!held || lent) {
				if (am.requestAudioFocus(focus) == AudioManager.AUDIOFOCUS_REQUEST_GRANTED) {
					held = true;
					lent = false;
				} else {
					// A call has the focus: the program stays
					// paused, and plays again when a lent focus
					// comes back.
					Native.media(2, 0);
				}
			}
		} else if (held && (!lent || st == null)) {
			am.abandonAudioFocusRequest(focus);
			held = false;
			lent = false;
		}
	}

	/** listen starts or stops listening for the sound to go to the speaker. */
	private static void listen(Context app, boolean on) {
		if (on == listening) {
			return;
		}
		listening = on;
		if (on) {
			app.registerReceiver(noisy, new IntentFilter(AudioManager.ACTION_AUDIO_BECOMING_NOISY));
		} else {
			app.unregisterReceiver(noisy);
		}
	}

	/**
	 * Focus hands the focus's changes to focusChanged. Android tells
	 * requests apart by their listener's toString, which for a lambda
	 * is the same in every gunim program, so it names the package.
	 */
	private static final class Focus implements AudioManager.OnAudioFocusChangeListener {
		private final String pkg;

		Focus(String pkg) {
			this.pkg = pkg;
		}

		@Override
		public void onAudioFocusChange(int change) {
			focusChanged(change);
		}

		@Override
		public String toString() {
			return "gunim.focus:" + pkg;
		}
	}

	/**
	 * focusChanged pauses for another player, which keeps the focus, and
	 * for a call or another short sound, which lends it; the program
	 * plays again when a lent focus comes back. Android lowers the
	 * program's volume itself for a sound that only asks it to duck.
	 */
	private static void focusChanged(int change) {
		switch (change) {
		case AudioManager.AUDIOFOCUS_LOSS:
			Native.app.getSystemService(AudioManager.class).abandonAudioFocusRequest(focus);
			held = false;
			lent = false;
			Native.media(2, 0);
			break;
		case AudioManager.AUDIOFOCUS_LOSS_TRANSIENT:
			if (state != null && state.playing) {
				lent = true;
				Native.media(2, 0);
			}
			break;
		case AudioManager.AUDIOFOCUS_GAIN:
			if (lent) {
				lent = false;
				Native.media(1, 0);
			}
			break;
		}
	}

	@Override
	public void onCreate() {
		super.onCreate();
		session = new MediaSession(this, "gunim");
		session.setCallback(new MediaSession.Callback() {
			@Override
			public void onPlay() {
				Native.media(1, 0);
			}

			@Override
			public void onPause() {
				Native.media(2, 0);
			}

			@Override
			public void onSkipToNext() {
				Native.media(4, 0);
			}

			@Override
			public void onSkipToPrevious() {
				Native.media(5, 0);
			}

			@Override
			public void onStop() {
				Native.media(6, 0);
			}

			@Override
			public void onSeekTo(long ms) {
				Native.media(7, ms);
			}
		});
		session.setActive(true);
		running = this;
	}

	@Override
	public IBinder onBind(Intent i) {
		return null;
	}

	@Override
	public int onStartCommand(Intent i, int flags, int startId) {
		// A start must put the service in the foreground, a button's
		// included; apply takes it out again while paused.
		startForeground(ID, notification());
		String a = i != null ? i.getAction() : null;
		if (PLAY.equals(a)) {
			Native.media(1, 0);
		} else if (PAUSE.equals(a)) {
			Native.media(2, 0);
		} else if (NEXT.equals(a)) {
			Native.media(4, 0);
		} else if (PREVIOUS.equals(a)) {
			Native.media(5, 0);
		}
		apply();
		return START_NOT_STICKY;
	}

	@Override
	public void onDestroy() {
		if (running == this) {
			running = null;
		}
		session.setActive(false);
		session.release();
		super.onDestroy();
	}

	/**
	 * apply shows the state in the session and the notification: in the
	 * foreground while it plays, and only shown while it is paused, so
	 * Android may let the program go.
	 */
	private void apply() {
		State st = state;
		if (st == null) {
			stopSelf();
			return;
		}
		MediaMetadata.Builder m = new MediaMetadata.Builder()
			.putString(MediaMetadata.METADATA_KEY_TITLE, st.title)
			.putString(MediaMetadata.METADATA_KEY_ARTIST, st.artist)
			.putString(MediaMetadata.METADATA_KEY_ALBUM, st.album);
		if (st.length > 0) {
			m.putLong(MediaMetadata.METADATA_KEY_DURATION, st.length);
		}
		if (st.art != null) {
			m.putBitmap(MediaMetadata.METADATA_KEY_ALBUM_ART, st.art);
		}
		session.setMetadata(m.build());
		session.setPlaybackState(new PlaybackState.Builder()
			.setActions(PlaybackState.ACTION_PLAY | PlaybackState.ACTION_PAUSE | PlaybackState.ACTION_PLAY_PAUSE
				| PlaybackState.ACTION_SKIP_TO_NEXT | PlaybackState.ACTION_SKIP_TO_PREVIOUS
				| PlaybackState.ACTION_SEEK_TO | PlaybackState.ACTION_STOP)
			.setState(st.playing ? PlaybackState.STATE_PLAYING : PlaybackState.STATE_PAUSED, st.position,
				st.playing ? 1f : 0f)
			.build());
		Notification n = notification();
		if (st.playing) {
			if (android.os.Build.VERSION.SDK_INT >= 29) {
				startForeground(ID, n, android.content.pm.ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PLAYBACK);
			} else {
				startForeground(ID, n);
			}
		} else {
			stopForeground(STOP_FOREGROUND_DETACH);
			getSystemService(NotificationManager.class).notify(ID, n);
		}
	}

	private Notification notification() {
		NotificationManager nm = getSystemService(NotificationManager.class);
		if (nm.getNotificationChannel(CHANNEL) == null) {
			CharSequence name = getApplicationInfo().loadLabel(getPackageManager());
			NotificationChannel c = new NotificationChannel(CHANNEL, name, NotificationManager.IMPORTANCE_LOW);
			c.setShowBadge(false);
			nm.createNotificationChannel(c);
		}
		State st = state;
		boolean playing = st != null && st.playing;
		Intent open = getPackageManager().getLaunchIntentForPackage(getPackageName());
		Notification.Builder b = new Notification.Builder(this, CHANNEL)
			.setSmallIcon(android.R.drawable.ic_media_play)
			.setContentTitle(st != null ? st.title : "")
			.setContentText(st != null ? st.artist : "")
			.setContentIntent(PendingIntent.getActivity(this, 0, open,
				PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT))
			.setOngoing(playing)
			.setVisibility(Notification.VISIBILITY_PUBLIC)
			.setCategory(Notification.CATEGORY_TRANSPORT)
			.addAction(action(android.R.drawable.ic_media_previous, "Previous", PREVIOUS))
			.addAction(playing ? action(android.R.drawable.ic_media_pause, "Pause", PAUSE)
				: action(android.R.drawable.ic_media_play, "Play", PLAY))
			.addAction(action(android.R.drawable.ic_media_next, "Next", NEXT))
			.setStyle(new Notification.MediaStyle()
				.setMediaSession(session.getSessionToken())
				.setShowActionsInCompactView(0, 1, 2));
		if (st != null && st.art != null) {
			b.setLargeIcon(st.art);
		}
		return b.build();
	}

	private Notification.Action action(int icon, String title, String what) {
		Intent i = new Intent(this, GunimService.class).setAction(what);
		PendingIntent p = PendingIntent.getForegroundService(this, what.hashCode(), i, PendingIntent.FLAG_IMMUTABLE);
		return new Notification.Action.Builder(android.graphics.drawable.Icon.createWithResource(this, icon), title, p).build();
	}
}
