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
	int multiline, int secret, long long seq);
void gunim_clear_text_state(void);
uint16_t *gunim_get_clipboard(int *n);
void gunim_set_clipboard(const uint16_t *s, int n);
void gunim_finish(void);
void gunim_ended(void);
void gunim_buzz(void);
void gunim_take_fingers(int on);
void gunim_keep_awake(int on);
int gunim_share(const uint16_t *text, int nt, const uint16_t *subject, int ns, const uint16_t *paths, int np);
int gunim_vibrate(const long long *ms, int n);
void gunim_exclude_gestures(const int *xy, int n);
int gunim_has_compass(void);
void gunim_watch_heading(int on);
int gunim_open_link(const uint16_t *url, int n);
int gunim_permitted(int p);
void gunim_ask(int p, int code);
void gunim_choose_folder(int code);
void gunim_choose_files(int code, int multiple, const uint16_t *patterns, int n);
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
