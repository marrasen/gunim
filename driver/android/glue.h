// The C half of the Android driver: the JNI entry points Java calls,
// the calls back into Java, and EGL. The Go half is in jni.go.

#pragma once

#include <stdint.h>
#include <sys/socket.h>
#include <android/native_window.h>

// Calls into Java, from any thread.
void gunim_show_keyboard(int show);
void gunim_now_playing(int on, int playing, const uint16_t *title, int nt, const uint16_t *artist, int na,
	const uint16_t *album, int nl, long long length, long long position, const void *art, int nart);
void gunim_caret(int x0, int y0, int x1, int y1);
void gunim_text_state(const uint16_t *text, int n, int start, int selA, int selB, int compA, int compB,
	int multiline, int secret, int raw, long long seq);
void gunim_clear_text_state(void);
uint16_t *gunim_get_clipboard(int *n);
void gunim_set_clipboard(const uint16_t *s, int n);
int gunim_set_clipboard_sensitive(const uint16_t *s, int n);
// gunim_clear_clipboard_if returns 1 where it emptied the clipboard, 0
// where that holds something else, or 2 where Android does not tell.
int gunim_clear_clipboard_if(const uint16_t *s, int n);
// gunim_secure returns 0, or 1 where other applications' windows may stay
// over the activity's.
int gunim_secure(int on);
void gunim_finish(void);
void gunim_ended(void);
void gunim_buzz(void);
void gunim_take_fingers(int on);
void gunim_keep_awake(int on);
int gunim_share(const uint16_t *text, int nt, const uint16_t *subject, int ns, const uint16_t *paths, int np,
	const long long *ids, const long long *sizes, int n, const uint16_t *served, int nd);
// gunim_open_served returns 0, 1 where nothing opens the file, or 2 where
// there is no activity to open it from.
int gunim_open_served(long long id, const uint16_t *name, int nn, const uint16_t *type, int nt, long long size);
void gunim_stop_served(long long id);
void gunim_create_document(int code, const uint16_t *name, int nn, const uint16_t *type, int nt);
void gunim_save_folder(int code);
// gunim_save does one of GunimSave's calls, with five strings, and returns
// what it says, n UTF-16 units of it for the caller to free.
uint16_t *gunim_save(int what, const uint16_t *const *s, const int *ns, int *n);
// gunim_notify shows a message of eight strings, and returns 0, 1 where
// the user has it off, or 2 where Android does not take it.
int gunim_notify(uint16_t *const *s, const int *ns, int importance, int lock, int parts, int ongoing, const void *icon,
	int nicon);
int gunim_notify_on(const uint16_t *channel, int n);
void gunim_notify_end(const uint16_t *tag, int n);
// gunim_watch_network returns 0 where Android refuses.
int gunim_watch_network(int on);
int gunim_vibrate(const long long *ms, int n);
void gunim_exclude_gestures(const int *xy, int n);
int gunim_has_compass(void);
void gunim_watch_heading(int on);
int gunim_open_link(const uint16_t *url, int n);
int gunim_permitted(int p);
void gunim_ask(int p, int code);
void gunim_choose_folder(int code);
void gunim_choose_files(int code, int multiple, const uint16_t *patterns, int n, int streams);
int gunim_stream_end(long long id, int wait);
void gunim_close_stream(long long id);
uint16_t *gunim_user_folder(int f, int *n);
uint16_t *gunim_no_backup_dir(int *n);
// gunim_env has goEnv run h with the thread's JNIEnv, and returns 0 where
// there is no Java. *thrown is the exception left, n UTF-16 units of it
// for the caller to free, or NULL. It returns 2 for one that says nothing
// of itself.
int gunim_env(uintptr_t h, uint16_t **thrown, int *n);

// Networks, by the handles Java numbers them with; 0 is the default.
// gunim_bind_socket keeps fd to one, and returns 0 or the errno.
// gunim_lookup looks host up on one, writes up to max addresses and
// returns how many; where it fails, *kind says how and failed holds what
// the system says.
typedef struct {
	unsigned char b[16];
	int n;
	uint32_t scope;
} gunim_addr;
enum { GUNIM_FAILED = 1, GUNIM_NO_NAME, GUNIM_TRY_AGAIN };
int gunim_bind_socket(long long network, int fd);
int gunim_lookup(long long network, const char *host, int family, gunim_addr *out, int max, char *failed, int nfailed,
	int *kind);

// For package audio/internal/aaudio, which calls them where the driver
// is linked in: what GunimAudio.route and GunimAudio.probe return, into
// r, n ints at most. Each returns how many it wrote, or 0.
int gunim_audio_route(int device, int *r, int n);
int gunim_audio_probe(int usage, int *r, int n);
// gunim_audio_watch has GunimAudio tell of audio devices coming and
// going, and returns 1 once it does; gunim_audio_changes writes how many
// times it told of an output, and of an input.
int gunim_audio_watch(void);
void gunim_audio_changes(long long *out, long long *in);

// EGL, from the render thread. Each returns 0, or the EGL error.
int gunim_egl_init(void);
int gunim_egl_attach(ANativeWindow *w);
void gunim_egl_detach(void);
int gunim_egl_swap(void);
// gunim_egl_stamps says the surface keeps frame timestamps, and
// gunim_egl_lag returns how long the last frame they told of took from
// its swap's return to the screen, in nanoseconds, or -1 before one has.
int gunim_egl_stamps(void);
long long gunim_egl_lag(void);
void gunim_window_release(ANativeWindow *w);

// gunim_log writes a line to the system log.
void gunim_log(const char *line);
// gunim_log_open has the system's own way to the log open.
void gunim_log_open(void);
// gunim_waits reports whether fd is one whose calls wait.
int gunim_waits(int fd);
// gunim_watch_crash writes a fixed line to the system log once anything
// comes through fd, and reads fd for as long as the process lives.
int gunim_watch_crash(int fd);
