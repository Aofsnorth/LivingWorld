import net.minecraft.SharedConstants;
import net.minecraft.core.Holder;
import net.minecraft.core.registries.Registries;
import net.minecraft.resources.Identifier;
import net.minecraft.resources.ResourceKey;
import net.minecraft.server.Bootstrap;
import net.minecraft.util.RandomSource;
import net.minecraft.world.level.levelgen.NoiseGeneratorSettings;
import net.minecraft.world.level.levelgen.Noises;
import net.minecraft.world.level.levelgen.RandomState;
import net.minecraft.world.level.levelgen.synth.Noise;

import java.lang.reflect.Field;

/**
 * PerlinProbe — prints the seeded internals of one vanilla 26.3 noise so the Go
 * port can be matched link by link: the positional factory draws, the per-octave
 * seed streams, and the resulting gradient-noise offsets/permutation.
 *
 * Usage: PerlinProbe <seed> <noiseId> <sampleX> <sampleY> <sampleZ>
 */
public class PerlinProbe {
    public static void main(String[] args) throws Exception {
        long seed = Long.parseLong(args[0]);
        String noiseId = args[1];
        int x = Integer.parseInt(args[2]), y = Integer.parseInt(args[3]), z = Integer.parseInt(args[4]);

        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();
        var lookup = net.minecraft.data.registries.VanillaRegistries.createWorldLookup();
        Holder<NoiseGeneratorSettings> settings = lookup.lookupOrThrow(Registries.NOISE_SETTINGS)
                .getOrThrow(ResourceKey.create(Registries.NOISE_SETTINGS, Identifier.parse("minecraft:overworld")));
        RandomState state = RandomState.create(lookup.lookupOrThrow(Registries.NOISE), seed, settings.value());

        var factory = Noises.class;
        Field prfF = RandomState.class.getDeclaredField("random");
        prfF.setAccessible(true);
        Object prf = prfF.get(state);
        System.out.println("world positional factory: " + factoryFields(prf));

        // The per-noise random is factory.fromHashOf(noiseId) — exactly what
        // Noises.instantiate does.
        RandomSource noiseRandom = invokeFromHashOf(prf, noiseId);
        System.out.println("noise random from " + noiseId + " = " + stateOf(noiseRandom));

        Object f1 = noiseRandom.forkPositional();
        Object f2 = noiseRandom.forkPositional();
        System.out.println("fork 1: " + factoryFields(f1));
        System.out.println("fork 2: " + factoryFields(f2));

        for (String octaveSeed : new String[]{"octave_-9", "octave_-8", "octave_-7", "octave_-6"}) {
            RandomSource r1 = invokeFromHashOf(f1, octaveSeed);
            RandomSource r2 = invokeFromHashOf(f2, octaveSeed);
            System.out.println(octaveSeed + " seed1=" + stateOf(r1));
            System.out.println(octaveSeed + " seed2=" + stateOf(r2));
        }

        Noise n = state.getOrCreateNoise(ResourceKey.create(Registries.NOISE, Identifier.parse(noiseId)));
        System.out.println(noiseId + " value at (" + x + "," + y + "," + z + ") = " + n.get(x, y, z));

        // The NoiseStack layers themselves, so the Go layer table can be diffed.
        Field layersF = Class.forName("net.minecraft.world.level.levelgen.synth.NoiseStack")
                .getDeclaredField("layers");
        layersF.setAccessible(true);
        Object[] layers = (Object[]) layersF.get(n);
        for (int i = 0; i < layers.length; i++) {
            Object layer = layers[i];
            Class<?> lc = layer.getClass();
            Field lf = lc.getDeclaredField("frequency"); lf.setAccessible(true);
            Field la = lc.getDeclaredField("amplitude"); la.setAccessible(true);
            Field ln = lc.getDeclaredField("noise"); ln.setAccessible(true);
            Object perlin = ln.get(layer);
            double freq = lf.getDouble(layer);
            float v = (Float) perlin.getClass().getMethod("get", double.class, double.class, double.class)
                    .invoke(perlin, x * freq, y * freq, z * freq);
            double amp = ((Float) la.get(layer)).doubleValue();
            System.out.printf("  layer[%d] freq=%.17g amp=%.9g sampled=%.9f contrib=%.9f perms0=%d%n",
                    i, freq, amp, v, amp * (double) v, firstPerm(perlin));
        }
    }

    private static int firstPerm(Object perlin) throws Exception {
        Class<?> gn = Class.forName("net.minecraft.world.level.levelgen.synth.GradientNoise");
        Field perms = gn.getDeclaredField("perms");
        perms.setAccessible(true);
        return ((byte[]) perms.get(perlin))[0] & 0xFF;
    }

    private static void dumpPerlinUnused() {
    }

    private static void dumpPerlin(String label, RandomSource r, int x, int y, int z) throws Exception {
        Class<?> perlinClass = Class.forName("net.minecraft.world.level.levelgen.synth.PerlinNoise");
        var ctor = perlinClass.getDeclaredConstructor(RandomSource.class);
        ctor.setAccessible(true);
        Object perlin = ctor.newInstance(r);
        Class<?> gn = Class.forName("net.minecraft.world.level.levelgen.synth.GradientNoise");
        Field ox = gn.getDeclaredField("offsetX"); ox.setAccessible(true);
        Field oy = gn.getDeclaredField("offsetY"); oy.setAccessible(true);
        Field oz = gn.getDeclaredField("offsetZ"); oz.setAccessible(true);
        Field perms = gn.getDeclaredField("perms"); perms.setAccessible(true);
        byte[] p = (byte[]) perms.get(perlin);
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < 12; i++) sb.append(p[i] & 0xFF).append(' ');
        var get = perlinClass.getMethod("get", double.class, double.class, double.class);
        System.out.printf("%s offsets=%.17g,%.17g,%.17g perms[0..11]=%s value=%s%n", label,
                (Double) ox.get(perlin), (Double) oy.get(perlin), (Double) oz.get(perlin), sb,
                get.invoke(perlin, (double) x, (double) y, (double) z));
    }

    private static String stateOf(RandomSource r) throws Exception {
        Field f = r.getClass().getDeclaredField("randomNumberGenerator");
        f.setAccessible(true);
        Object gen = f.get(r);
        Field lo = gen.getClass().getDeclaredField("seedLo");
        Field hi = gen.getClass().getDeclaredField("seedHi");
        lo.setAccessible(true);
        hi.setAccessible(true);
        return "lo=" + lo.getLong(gen) + " hi=" + hi.getLong(gen);
    }

    private static String factoryFields(Object prf) throws Exception {
        StringBuilder sb = new StringBuilder();
        for (Field f : prf.getClass().getDeclaredFields()) {
            if (f.getType() != long.class) continue;
            f.setAccessible(true);
            sb.append(f.getName()).append('=').append(f.getLong(prf)).append(' ');
        }
        return sb.toString();
    }

    private static RandomSource invokeFromHashOf(Object prf, String name) throws Exception {
        var m = prf.getClass().getMethod("fromHashOf", String.class);
        return (RandomSource) m.invoke(prf, name);
    }
}
