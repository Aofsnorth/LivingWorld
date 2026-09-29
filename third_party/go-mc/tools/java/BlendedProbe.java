import net.minecraft.SharedConstants;
import net.minecraft.core.Holder;
import net.minecraft.core.registries.Registries;
import net.minecraft.resources.Identifier;
import net.minecraft.resources.ResourceKey;
import net.minecraft.server.Bootstrap;
import net.minecraft.util.RandomSource;
import net.minecraft.world.level.levelgen.NoiseGeneratorSettings;
import net.minecraft.world.level.levelgen.RandomState;
import net.minecraft.world.level.levelgen.synth.BlendedNoise;
import net.minecraft.world.level.levelgen.synth.NoiseStack;

import java.lang.reflect.Field;
import java.lang.reflect.Method;

/**
 * BlendedProbe — prints the 26.3 BlendedNoise component values (minLimit,
 * maxLimit, main, clamped t, result) at a block position so the Go port can be
 * diffed component by component.
 *
 * Usage: BlendedProbe <seed> <xzScale> <yScale> <xzFactor> <yFactor> <smear> <x> <y> <z>
 */
public class BlendedProbe {
    public static void main(String[] args) throws Exception {
        long seed = Long.parseLong(args[0]);
        double xzScale = Double.parseDouble(args[1]);
        double yScale = Double.parseDouble(args[2]);
        double xzFactor = Double.parseDouble(args[3]);
        double yFactor = Double.parseDouble(args[4]);
        double smear = Double.parseDouble(args[5]);
        int x = Integer.parseInt(args[6]), y = Integer.parseInt(args[7]), z = Integer.parseInt(args[8]);

        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();
        var lookup = net.minecraft.data.registries.VanillaRegistries.createWorldLookup();
        Holder<NoiseGeneratorSettings> settings = lookup.lookupOrThrow(Registries.NOISE_SETTINGS)
                .getOrThrow(ResourceKey.create(Registries.NOISE_SETTINGS, Identifier.parse("minecraft:overworld")));
        RandomState state = RandomState.create(lookup.lookupOrThrow(Registries.NOISE), seed, settings.value());

        // RandomState$1.createRandom(id) == RandomState.random.fromHashOf(id),
        // where RandomState.random is the world positional factory.
        Field prfField = RandomState.class.getDeclaredField("random");
        prfField.setAccessible(true);
        var factory = (net.minecraft.world.level.levelgen.PositionalRandomFactory) prfField.get(state);
        RandomSource random = factory.at(0, 0, 0);
        Method fromHashOf = factory.getClass().getMethod("fromHashOf", String.class);
        Method fromSeed = factory.getClass().getMethod("fromSeed", long.class);

        BlendedNoise bn = new BlendedNoise(xzScale, yScale, xzFactor, yFactor, smear);
        Method xzM = BlendedNoise.class.getDeclaredMethod("xzMultiplier");
        Method yM = BlendedNoise.class.getDeclaredMethod("yMultiplier");
        xzM.setAccessible(true);
        yM.setAccessible(true);
        double xzm = (Double) xzM.invoke(bn);
        double ym = (Double) yM.invoke(bn);

        // Try the plausible random derivations; the one that reproduces the
        // TreeOracle value is the real one.
        record Candidate(String label, RandomSource random) {}
        var candidates = new java.util.ArrayList<Candidate>();
        candidates.add(new Candidate("at(0,0,0)", factory.at(0, 0, 0)));
        candidates.add(new Candidate("fromHashOf(minecraft:terrain)", (RandomSource) fromHashOf.invoke(factory, "minecraft:terrain")));
        candidates.add(new Candidate("fromHashOf(terrain)", (RandomSource) fromHashOf.invoke(factory, "terrain")));
        candidates.add(new Candidate("fromSeed(seed)", (RandomSource) fromSeed.invoke(factory, seed)));

        for (var c : candidates) {
            if (!c.label().equals("fromHashOf(minecraft:terrain)")) continue;
            var set = bn.createFbmSet(c.random());
            dumpStack("minLimit", set.minLimitNoise(), x * xzm, y * ym, z * xzm);
            dumpStack("main", set.mainNoise(), x * (xzm / xzFactor), y * (ym / yFactor), z * (xzm / xzFactor));
            float minLimit = set.minLimitNoise().get(x * xzm, y * ym, z * xzm);
            float maxLimit = set.maxLimitNoise().get(x * xzm, y * ym, z * xzm);
            float main = set.mainNoise().get(x * (xzm / xzFactor), y * (ym / yFactor), z * (xzm / xzFactor));
            float t = Math.min(1.0F, Math.max(0.0F, main + 0.5F));
            float result = minLimit + t * (maxLimit - minLimit);
            System.out.printf("%-32s minLimit=%.9g maxLimit=%.9g main=%.9g t=%.9g result=%.9g%n",
                    c.label(), minLimit, maxLimit, main, t, result);
        }
    }

    private static void dumpStack(String label, NoiseStack stack, double x, double y, double z) throws Exception {
        Field layersF = NoiseStack.class.getDeclaredField("layers");
        layersF.setAccessible(true);
        Object[] layers = (Object[]) layersF.get(stack);
        for (int i = 0; i < layers.length; i++) {
            Class<?> lc = layers[i].getClass();
            Field lf = lc.getDeclaredField("frequency"); lf.setAccessible(true);
            Field la = lc.getDeclaredField("amplitude"); la.setAccessible(true);
            Field ln = lc.getDeclaredField("noise"); ln.setAccessible(true);
            Object perlin = ln.get(layers[i]);
            double freq = lf.getDouble(layers[i]);
            double amp = ((Float) la.get(layers[i])).doubleValue();
            float v = (Float) perlin.getClass().getMethod("get", double.class, double.class, double.class)
                    .invoke(perlin, x * freq, y * freq, z * freq);
            System.out.printf("  %s layer[%d] freq=%.17g amp=%.9g sampled=%.9f%n", label, i, freq, amp, v);
            if (i < 3) {
                Field fudgeF = perlin.getClass().getDeclaredField("fudgeYScale");
                fudgeF.setAccessible(true);
                Field permsF = Class.forName("net.minecraft.world.level.levelgen.synth.GradientNoise")
                        .getDeclaredField("perms");
                permsF.setAccessible(true);
                byte[] perms = (byte[]) permsF.get(perlin);
                Field oxF = Class.forName("net.minecraft.world.level.levelgen.synth.GradientNoise")
                        .getDeclaredField("offsetX");
                oxF.setAccessible(true);
                System.out.printf("      fudgeYScale=%.17g perms0..3=%d %d %d %d offsetX=%.17g%n",
                        (Double) fudgeF.get(perlin), perms[0] & 0xFF, perms[1] & 0xFF,
                        perms[2] & 0xFF, perms[3] & 0xFF, (Double) oxF.get(perlin));
            }
        }
    }
}
