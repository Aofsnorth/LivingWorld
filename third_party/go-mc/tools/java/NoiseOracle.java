import net.minecraft.SharedConstants;
import net.minecraft.core.Holder;
import net.minecraft.core.registries.Registries;
import net.minecraft.resources.Identifier;
import net.minecraft.resources.ResourceKey;
import net.minecraft.server.Bootstrap;
import net.minecraft.world.level.levelgen.RandomState;
import net.minecraft.world.level.levelgen.NoiseGeneratorSettings;
import net.minecraft.world.level.levelgen.synth.NormalNoise;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;

/**
 * NoiseOracle — raw vanilla 26.3 NormalNoise values, so LivingWorld's Perlin /
 * octave-sum implementation can be compared to the real thing value by value.
 *
 * Usage: NoiseOracle <seed> <in.txt> <out.txt>
 *   in.txt : "x y z" per line
 *   out.txt: per line "<noiseId> <x> <y> <z> <value>" for every listed noise
 */
public class NoiseOracle {
    static final List<String> NOISES = List.of(
            "minecraft:continentalness", "minecraft:temperature", "minecraft:vegetation",
            "minecraft:erosion", "minecraft:continentalness_large", "minecraft:erosion_large",
            "minecraft:ridge", "minecraft:calcite");

    public static void main(String[] args) throws Exception {
        if (args.length < 3) {
            System.out.println("usage: NoiseOracle <seed> <in.txt> <out.txt>");
            System.exit(2);
        }
        long seed = Long.parseLong(args[0]);
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();

        var lookup = net.minecraft.data.registries.VanillaRegistries.createWorldLookup();
        Holder<NoiseGeneratorSettings> settings = lookup.lookupOrThrow(Registries.NOISE_SETTINGS)
                .getOrThrow(ResourceKey.create(Registries.NOISE_SETTINGS, Identifier.parse("minecraft:overworld")));
        RandomState state = RandomState.create(lookup.lookupOrThrow(Registries.NOISE), seed, settings.value());

        StringBuilder sb = new StringBuilder();
        for (String line : Files.readAllLines(Path.of(args[1]))) {
            String[] p = line.trim().split("\\s+");
            if (p.length < 3) continue;
            int x = Integer.parseInt(p[0]), y = Integer.parseInt(p[1]), z = Integer.parseInt(p[2]);
            for (String id : NOISES) {
                net.minecraft.world.level.levelgen.synth.Noise noise = state.getOrCreateNoise(
                        ResourceKey.create(Registries.NOISE, Identifier.parse(id)));
                sb.append(id).append(' ').append(x).append(' ').append(y).append(' ').append(z)
                        .append(' ').append(noise.get(x, y, z)).append('\n');
            }
        }
        Files.write(Path.of(args[2]), sb.toString().getBytes(StandardCharsets.UTF_8));
        System.out.println("noise oracle done");
    }
}
