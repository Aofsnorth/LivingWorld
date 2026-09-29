import net.minecraft.SharedConstants;
import net.minecraft.core.Holder;
import net.minecraft.core.registries.Registries;
import net.minecraft.resources.Identifier;
import net.minecraft.resources.ResourceKey;
import net.minecraft.server.Bootstrap;
import net.minecraft.world.level.levelgen.NoiseGeneratorSettings;
import net.minecraft.world.level.levelgen.RandomState;
import net.minecraft.world.level.levelgen.densityfunction.DensityFunction;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;

/**
 * TreeOracle — samples named vanilla 26.3 density functions at block
 * coordinates, so LivingWorld's parsed tree can be diffed node by node and the
 * first diverging sub-function located.
 *
 * Output: one line "<fn>\t<x>\t<y>\t<z>\t<value>".
 *
 * Usage: TreeOracle <seed> <in.txt> <out.txt> [fn ids...]
 */
public class TreeOracle {
    public static void main(String[] args) throws Exception {
        if (args.length < 3) {
            System.out.println("usage: TreeOracle <seed> <in.txt> <out.txt> [fn ids...]");
            System.exit(2);
        }
        long seed = Long.parseLong(args[0]);
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();

        var lookup = net.minecraft.data.registries.VanillaRegistries.createWorldLookup();
        Holder<NoiseGeneratorSettings> settings = lookup.lookupOrThrow(Registries.NOISE_SETTINGS)
                .getOrThrow(ResourceKey.create(Registries.NOISE_SETTINGS, Identifier.parse("minecraft:overworld")));
        RandomState randomState = RandomState.create(lookup.lookupOrThrow(Registries.NOISE), seed, settings.value());
        var dfLookup = lookup.lookupOrThrow(Registries.DENSITY_FUNCTION);

        List<String> ids = new ArrayList<>();
        for (int i = 3; i < args.length; i++) ids.add(args[i]);
        if (ids.isEmpty()) {
            // Every density function the jar knows, so the first diverging
            // sub-function shows up directly.
            lookup.lookupOrThrow(Registries.DENSITY_FUNCTION).listElementIds()
                    .forEach(k -> ids.add(k.identifier().toString()));
        }

        List<DensityFunction> fns = new ArrayList<>();
        for (String id : ids) {
            fns.add(dfLookup.getOrThrow(ResourceKey.create(Registries.DENSITY_FUNCTION, Identifier.parse(id))).value());
        }

        StringBuilder sb = new StringBuilder();
        for (String line : Files.readAllLines(Path.of(args[1]))) {
            String[] p = line.trim().split("\\s+");
            if (p.length < 3) continue;
            int x = Integer.parseInt(p[0]), y = Integer.parseInt(p[1]), z = Integer.parseInt(p[2]);
            for (int i = 0; i < ids.size(); i++) {
                float v = randomState.sampleBlockValueUncached(fns.get(i), x, y, z);
                sb.append(ids.get(i)).append('\t').append(x).append('\t').append(y).append('\t').append(z)
                        .append('\t').append(v).append('\n');
            }
        }
        Files.write(Path.of(args[2]), sb.toString().getBytes(StandardCharsets.UTF_8));
        System.out.println("tree oracle done: " + ids.size() + " functions");
    }
}
