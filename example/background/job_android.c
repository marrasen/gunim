// The C half of the job: what Java's Work calls, and what Go calls of it.

#include <jni.h>
#include "_cgo_export.h"

JNIEXPORT void JNICALL Java_org_gunim_background_Work_started(JNIEnv *env, jclass c, jlong network) {
	workStarted(network);
}

JNIEXPORT void JNICALL Java_org_gunim_background_Work_stopped(JNIEnv *env, jclass c) {
	workStopped();
}

int schedule_work(JNIEnv *env, jobject context) {
	jclass c = (*env)->FindClass(env, "org/gunim/background/Work");
	if (c == NULL) {
		return 0;
	}
	jmethodID m = (*env)->GetStaticMethodID(env, c, "schedule", "(Landroid/content/Context;)Z");
	return m != NULL && (*env)->CallStaticBooleanMethod(env, c, m, context);
}

void finish_work(JNIEnv *env) {
	jclass c = (*env)->FindClass(env, "org/gunim/background/Work");
	if (c == NULL) {
		return;
	}
	jmethodID m = (*env)->GetStaticMethodID(env, c, "finished", "()V");
	if (m != NULL) {
		(*env)->CallStaticVoidMethod(env, c, m);
	}
}
