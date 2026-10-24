# The gomobile engine is called through JNI by name: keep its Java side and the generated bindings.
-keep class go.** { *; }
-keep class systems.darkcipher.parallelpath.gocore.** { *; }
# kotlinx.serialization: keep generated serializers of the model classes.
-keepclassmembers class systems.darkcipher.parallelpath.model.** { *** Companion; *** serializer(...); }
-keep class systems.darkcipher.parallelpath.model.**$$serializer { *; }
-dontwarn java.lang.invoke.StringConcatFactory
