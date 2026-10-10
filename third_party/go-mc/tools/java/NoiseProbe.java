import net.minecraft.SharedConstants;
import net.minecraft.core.Holder;
import net.minecraft.core.registries.Registries;
import net.minecraft.resources.Identifier;
import net.minecraft.resources.ResourceKey;
import net.minecraft.server.Bootstrap;
import net.minecraft.world.level.levelgen.NoiseGeneratorSettings;
import net.minecraft.world.level.levelgen.RandomState;
import net.minecraft.world.level.levelgen.synth.NormalNoise;

import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.util.List;

/**
 * NoiseProbe — prints the 26.3 NormalNoise internals (octave list, per-octave
 * seed strings, frequencies, amplitudes, normalization factor) so the Go port
 * can be written against the real values instead of guessed constants.
 *
 * Usage: NoiseProbe <seed> [noise ids...]
 */
public class NoiseProbe {
    public static void main(String[] args) throws Exception {
        long seed = Long.parseLong(args[0]);
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();

        var lookup = net.minecraft.data.registries.VanillaRegistries.createWorldLookup();
        Holder<NoiseGeneratorSettings> settings = lookup.lookupOrThrow(Registries.NOISE_SETTINGS)
                .getOrThrow(ResourceKey.create(Registries.NOISE_SETTINGS, Identifier.parse("minecraft:overworld")));
        RandomState state = RandomState.create(lookup.lookupOrThrow(Registries.NOISE), seed, settings.value());

        String[] ids = args.length > 1 ? java.util.Arrays.copyOfRange(args, 1, args.length)
                : new String[]{"minecraft:continentalness", "minecraft:temperature", "minecraft:vegetation",
                "minecraft:erosion", "minecraft:continentalness_large", "minecraft:erosion_large",
                "minecraft:ridge", "minecraft:calcite"};

        Field octavesF = NormalNoise.class.getDeclaredField("octaves");
        octavesF.setAccessible(true);
        Field normF = NormalNoise.class.getDeclaredField("normalizationFactor");
        normF.setAccessible(true);
        Field paramsF = NormalNoise.class.getDeclaredField("parameters");
        paramsF.setAccessible(true);
        Field prfF = RandomState.class.getDeclaredField("random");
        prfF.setAccessible(true);
        Object factory = prfF.get(state);
        System.out.println("RandomState positional factory: " + factory.getClass().getName());
        for (Field f : factory.getClass().getDeclaredFields()) {
            f.setAccessible(true);
            System.out.println("  " + f.getName() + " = " + f.get(factory));
        }

        for (String id : ids) {
            NormalNoise n = lookup.lookupOrThrow(Registries.NOISE)
                    .getOrThrow(ResourceKey.create(Registries.NOISE, Identifier.parse(id)))
                    .value();
            System.out.println("--- " + id);
            System.out.println("  parameters: " + paramsF.get(n));
            System.out.println("  normalizationFactor = " + normF.get(n));
            @SuppressWarnings("unchecked")
            List<Object> octaves = (List<Object>) octavesF.get(n);
            for (Object o : octaves) {
                Class<?> c = o.getClass();
                Field oi = c.getDeclaredField("octaveIndex"); oi.setAccessible(true);
                Field fr = c.getDeclaredField("frequency"); fr.setAccessible(true);
                Field am = c.getDeclaredField("amplitude"); am.setAccessible(true);
                Method m = c.getMethod("seed");
                m.setAccessible(true);
                System.out.printf("  octave index=%s frequency=%.17g amplitude=%.17g seed=%s%n",
                        oi.get(o), fr.getDouble(o), am.getDouble(o), m.invoke(o));
            }
        }
    }
}
